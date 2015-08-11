package unexport

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build"
	"go/format"
	"go/token"
	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/types"
	"golang.org/x/tools/refactor/importgraph"
	"io/ioutil"
	"log"
)

var (
	Verbose bool
)

func (u *Unexporter) unusedObjects() []types.Object {
	used := u.usedObjects()
	objs := make(map[string][]types.Object)
	for _, pkgInfo := range u.packages {
		for id, obj := range pkgInfo.Defs {
			if used[obj] {
				continue
			}
			if id.IsExported() {
				objs[pkgInfo.Pkg.Path()] = append(objs[pkgInfo.Pkg.Path()], obj)
			}
		}
	}
	return objs[u.path]
}

func (u *Unexporter) usedObjects() map[types.Object]bool {
	objs := make(map[types.Object]bool)
	for _, pkgInfo := range u.packages {
		// easy path
		for id, obj := range pkgInfo.Uses {
			// ignore builtin value
			if obj.Pkg() == nil {
				continue
			}
			// if it's a type from different package, store it
			if obj.Pkg() != pkgInfo.Pkg {
				objs[obj] = true
			}
			// embedded field
			if field := pkgInfo.Defs[id]; field != nil {
				// embdded field identifier is the same as it's type
				objs[field] = true
			}
		}
	}
	for key := range u.satisfy() {
		var (
			lhs, rhs *types.Named
			ok       bool
		)
		if lhs, ok = key.LHS.(*types.Named); !ok {
			continue
		}
		if rhs, ok = key.RHS.(*types.Named); !ok {
			continue
		}
		// if satisfied by type within the same package only, it should not be exported
		if lhs.Obj().Pkg() == rhs.Obj().Pkg() {
			continue
		}
		lset := u.msets.MethodSet(key.LHS)
		for i := 0; i < lset.Len(); i++ {
			obj := lset.At(i).Obj()
			objs[obj] = true
		}
		rset := u.msets.MethodSet(key.RHS)
		for i := 0; i < rset.Len(); i++ {
			obj := rset.At(i).Obj()
			objs[obj] = true
		}

	}
	return objs
}

func getDeclareStructOrInterface(prog *loader.Program, v *types.Var) string {
	// From x/tools/refactor/rename/check.go(checkStructField)#L288
	// go/types offers no easy way to get from a field (or interface
	// method) to its declaring struct (or interface), so we must
	// ascend the AST.
	_, path, _ := prog.PathEnclosingInterval(v.Pos(), v.Pos())
	// path matches this pattern:
	// [Ident SelectorExpr? StarExpr? Field FieldList StructType ParenExpr* ... File]

	// Ascend to FieldList.
	var i int
	for {
		if _, ok := path[i].(*ast.FieldList); ok {
			break
		}
		i++
	}
	i++
	_ = path[i].(*ast.StructType)
	i++
	for {
		if _, ok := path[i].(*ast.ParenExpr); !ok {
			break
		}
		i++
	}
	if spec, ok := path[i].(*ast.TypeSpec); ok {
		return spec.Name.String()
	}
	return ""
}

func loadProgram(ctx *build.Context, pkgs []string) (*loader.Program, error) {
	conf := loader.Config{
		Build: ctx,
	}
	for _, pkg := range pkgs {
		conf.Import(pkg)
	}
	return conf.Load()
}

// New creates a new Unexporter object that holds the states
func New(ctx *build.Context, path string) (*Unexporter, error) {
	pkgs := scanWorkspace(ctx, path)
	prog, err := loadProgram(ctx, pkgs)

	if err != nil {
		return nil, err
	}
	u := &Unexporter{
		path:         path,
		iprog:        prog,
		objsToUpdate: make(map[types.Object]map[types.Object]string),
		packages:     make(map[*types.Package]*loader.PackageInfo),
		warnings:     make(chan string),
		Identifiers:  make(map[types.Object]string),
	}

	for _, info := range prog.Imported {
		u.packages[info.Pkg] = info
	}

	for _, info := range prog.Created {
		u.packages[info.Pkg] = info
	}

	objs := make(chan map[types.Object]map[types.Object]string)
	unusedObjs := u.unusedObjects()
	for _, obj := range unusedObjs {
		toName := lowerFirst(obj.Name())
		go func(obj types.Object, toName string) {
			objsToUpdate := make(map[types.Object]string)
			u.check(objsToUpdate, obj, toName)
			objs <- map[types.Object]map[types.Object]string{obj: objsToUpdate}
		}(obj, toName)

		u.Identifiers[obj] = wholePath(obj, u.path, u.iprog)
	}
	// do it in another goroutine, or the write to u.warnings is blocked since it's a non-buffered channel
	go func() {
		for i := 0; i < len(unusedObjs); i++ {
			for obj, objsToUpdate := range <-objs {
				u.objsToUpdate[obj] = objsToUpdate
			}
		}
		close(objs)
		close(u.warnings)
	}()
	for warning := range u.warnings {
		log.Println(warning)
	}
	return u, nil
}

func (u *Unexporter) Update(obj types.Object) error {
	return u.update(u.objsToUpdate[obj])
}

func (u *Unexporter) UpdateAll() error {
	objsToUpdate := make(map[types.Object]string)
	for _, objs := range u.objsToUpdate {
		for obj, to := range objs {
			objsToUpdate[obj] = to
		}
	}
	return u.update(objsToUpdate)
}

// update renames the identifiers, updates the input files.
func (u *Unexporter) update(objsToUpdate map[types.Object]string) error {
	// We use token.File, not filename, since a file may appear to
	// belong to multiple packages and be parsed more than once.
	// token.File captures this distinction; filename does not.
	var nidents int
	var filesToUpdate = make(map[*token.File]bool)
	for _, info := range u.packages {
		// Mutate the ASTs and note the filenames.
		for id, obj := range info.Defs {
			if to, ok := objsToUpdate[obj]; ok {
				nidents++
				id.Name = to
				filesToUpdate[u.iprog.Fset.File(id.Pos())] = true
			}
		}
		for id, obj := range info.Uses {
			if to, ok := objsToUpdate[obj]; ok {
				nidents++
				id.Name = to
				filesToUpdate[u.iprog.Fset.File(id.Pos())] = true
			}
		}
	}

	// TODO(adonovan): don't rewrite cgo + generated files.
	var nerrs, npkgs int
	for _, info := range u.packages {
		first := true
		for _, f := range info.Files {
			tokenFile := u.iprog.Fset.File(f.Pos())
			if filesToUpdate[tokenFile] {
				if first {
					npkgs++
					first = false
					if Verbose {
						log.Printf("Updating package %s\n",
							info.Pkg.Path())
					}
				}
				if err := rewriteFile(u.iprog.Fset, f, tokenFile.Name()); err != nil {
					log.Printf("gorename: %s\n", err)
					nerrs++
				}
			}
		}
	}
	log.Printf("Renamed %d occurrence%s in %d file%s in %d package%s.\n",
		nidents, plural(nidents),
		len(filesToUpdate), plural(len(filesToUpdate)),
		npkgs, plural(npkgs))
	if nerrs > 0 {
		return fmt.Errorf("failed to rewrite %d file%s", nerrs, plural(nerrs))
	}
	return nil
}

func plural(n int) string {
	if n != 1 {
		return "s"
	}
	return ""
}

var rewriteFile = func(fset *token.FileSet, f *ast.File, filename string) (err error) {
	// TODO(adonovan): print packages and filenames in a form useful
	// to editors (so they can reload files).
	if Verbose {
		log.Printf("\t%s\n", filename)
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return fmt.Errorf("failed to pretty-print syntax tree: %v", err)
	}
	return ioutil.WriteFile(filename, buf.Bytes(), 0644)
}

func scanWorkspace(ctxt *build.Context, path string) []string {
	// Scan the workspace and build the import graph.
	_, rev, errors := importgraph.Build(ctxt)
	if len(errors) > 0 {
		// With a large GOPATH tree, errors are inevitable.
		// Report them but proceed.
		log.Printf("While scanning Go workspace:\n")
		for path, err := range errors {
			log.Printf("Package %q: %s.\n", path, err)
		}
	}

	// Enumerate the set of potentially affected packages.
	var affectedPackages []string
	// External test packages are never imported,
	// so they will never appear in the graph.
	for pkg := range rev.Search(path) {
		affectedPackages = append(affectedPackages, pkg)
	}
	return affectedPackages
}
