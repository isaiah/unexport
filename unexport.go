package unexport

import (
	"go/ast"
	"go/build"
	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/types"
	"golang.org/x/tools/refactor/importgraph"
	"log"
)

func (u *unexporter) unusedObjects() map[string][]types.Object {
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
	return objs
}

func (u *unexporter) usedObjects() map[types.Object]bool {
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

// Main The main entrance of the program, used by the CLI
func Main(ctx *build.Context, path string) (identifiers map[string]string, err error) {
	pkgs := scanWorkspace(ctx, path)
	prog, err := loadProgram(ctx, pkgs)

	if err != nil {
		return
	}
	u := &unexporter{
		iprog:        prog,
		objsToUpdate: make(map[types.Object]bool),
		packages:     make(map[*types.Package]*loader.PackageInfo),
		warnings:     make(chan string),
	}

	for _, info := range prog.Imported {
		u.packages[info.Pkg] = info
	}

	for _, info := range prog.Created {
		u.packages[info.Pkg] = info
	}

	identifiers = make(map[string]string)
	objs := make(chan map[types.Object]bool)
	unusedObjs := u.unusedObjects()[path]
	for _, obj := range unusedObjs {
		toName := lowerFirst(obj.Name())
		go func(obj types.Object, toName string) {
			objsToUpdate := make(map[types.Object]bool)
			u.check(objsToUpdate, obj, toName)
			objs <- objsToUpdate
		}(obj, toName)

		identifiers[wholePath(obj, path, prog)] = toName
	}
	// do it in another goroutine, or the write to u.warnings is blocked since it's a non-buffered channel
	go func() {
		for i := 0; i < len(unusedObjs); i++ {
			for obj, t := range <-objs {
				u.objsToUpdate[obj] = t
			}
		}
		close(objs)
		close(u.warnings)
	}()
	for warning := range u.warnings {
		log.Println(warning)
	}
	return
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
