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
	Usage   = `
	The unexport command first display the overall information, and highten the potential naming conflicts as a result of the renaming, then it prompt the interactive command line to allow confirm each renaming items. It also offers opportunity to change the resulting name, by default it only downcase the first letter of the original name.
	`
)

func (u *Unexporter) unusedObjects() []types.Object {
	used := u.usedObjects()
	var objs []types.Object
	for _, pkgInfo := range u.packages {
		if pkgInfo.Pkg.Path() != u.path {
			continue
		}
		for id, obj := range pkgInfo.Defs {
			if used[obj] {
				continue
			}
			if id.IsExported() {
				objs = append(objs, obj)
			}
		}
		// No need to go further if path found
		break
	}
	return objs
}

func (u *Unexporter) usedObjects() map[types.Object]bool {
	objs := make(map[types.Object]bool)
	for _, pkgInfo := range u.packages {
		// skip the target package
		if pkgInfo.Pkg.Path() == u.path {
			continue
		}
		// easy path
		for id, obj := range pkgInfo.Uses {
			// ignore builtin value
			if obj.Pkg() == nil {
				continue
			}
			// if it's a type from different package, store it
			// Only store objects from target package
			if obj.Pkg() != pkgInfo.Pkg {
				//obj.Pkg().Path() == u.path {
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
		// not interested if neither of the objects belong to the target package
		//if lhs.Obj().Pkg().Path() != u.path && rhs.Obj().Pkg().Path() != u.path {
		//	continue
		//}
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
		path:        path,
		iprog:       prog,
		packages:    make(map[*types.Package]*loader.PackageInfo),
		warnings:    make(chan map[types.Object]string),
		Identifiers: make(map[types.Object]*ObjectInfo),
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
		u.Identifiers[obj] = &ObjectInfo{}
	}
	for i := 0; i < len(unusedObjs); {
		select {
		case m := <-u.warnings:
			for obj, warning := range m {
				u.Identifiers[obj].Warning = warning
			}
		case m := <-objs:
			for obj, objsToUpdate := range m {
				u.Identifiers[obj].objsToUpdate = objsToUpdate
			}
			i++
		}
	}
DONE:
	for {
		select {
		case m := <-u.warnings:
			for obj, warning := range m {
				u.Identifiers[obj].Warning = warning
			}
		default:
			break DONE
		}
	}
	return u, nil
}

func (u *Unexporter) Update(obj types.Object) error {
	return u.update(u.Identifiers[obj].objsToUpdate)
}

func (u *Unexporter) UpdateAll() error {
	objsToUpdate := make(map[types.Object]string)
	for _, objInfo := range u.Identifiers {
		for obj, to := range objInfo.objsToUpdate {
			objsToUpdate[obj] = to
		}
	}
	return u.update(objsToUpdate)
}

func (u *Unexporter) Check(from types.Object, to string) string {
	objsToUpdate := make(map[types.Object]string)
	u.warnings = make(chan map[types.Object]string)
	u.check(objsToUpdate, from, to)
	close(u.warnings)
	u.Identifiers[from] = &ObjectInfo{objsToUpdate: objsToUpdate}
	for ws := range u.warnings {
		for _, warning := range ws {
			u.Identifiers[from].Warning = warning
			return warning
		}
	}
	return ""
}

func (u *Unexporter) Qualifier(obj types.Object) string {
	return wholePath(obj, u.path, u.iprog)
}

// This is copy & pasted from x/tools/refactor/rename
// this is here becuase the collision checking are already done, and the context are
// initialized, so that it doesn't have to go through the workspace to find all the relevant
// packages, that's the most expensive operation
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
