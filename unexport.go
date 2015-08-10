package unexport

import (
	"go/ast"
	"go/build"
	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/types"
	"golang.org/x/tools/go/types/typeutil"
	"golang.org/x/tools/refactor/satisfy"
	"log"
)

type unexporter struct {
	packages           map[string]*loader.PackageInfo
	satisfyConstraints map[satisfy.Constraint]bool
	msets              typeutil.MethodSetCache
}

// satisfy copied from x/tools/refactor/rename
// Find the satisfy relationship, all interface satisfied should be exported
func (u *unexporter) satisfy() map[satisfy.Constraint]bool {
	if u.satisfyConstraints == nil {
		var f satisfy.Finder
		for _, info := range u.packages {
			f.Find(&info.Info, info.Files)
		}
		u.satisfyConstraints = f.Result
	}
	return u.satisfyConstraints
}

func (u *unexporter) unusedObjects() map[*types.Package][]types.Object {
	used := u.usedObjects()
	objs := make(map[*types.Package][]types.Object)
	for _, pkgInfo := range u.packages {
		for id, obj := range pkgInfo.Defs {
			if used[obj] {
				continue
			}
			if id.IsExported() {
				objs[pkgInfo.Pkg] = append(objs[pkgInfo.Pkg], obj)
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
func Main(ctx *build.Context, pkgs []string) (identifiers map[string]string, err error) {
	prog, err := loadProgram(ctx, pkgs)

	if err != nil {
		return
	}
	u := &unexporter{packages: prog.Imported}

	identifiers = make(map[string]string)
	for pkg, objs := range u.unusedObjects() {
		for _, obj := range objs {
			identifiers[wholePath(obj, pkg, prog)] = lowerFirst(obj.Name())
		}
	}
	return
}

func printImported(prog *loader.Program) {
	for name, pkginfo := range prog.Imported {
		log.Printf("imported %s => %v\n", name, pkginfo)
	}
}

func printInitialPackages(prog *loader.Program) {
	for _, pkg := range prog.InitialPackages() {
		log.Printf("find package: %v", pkg)
		for id, obj := range pkg.Defs {
			if id.IsExported() {
				log.Printf("%v => %v\n", id, obj)
			}
		}
	}
}
