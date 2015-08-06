package unexport

import (
	"fmt"
	"go/ast"
	"go/build"
	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/types"
	"log"
)

func exportedObjects(prog *loader.Program) map[*types.Package][]types.Object {
	objs := make(map[*types.Package][]types.Object)
	for _, pkgInfo := range prog.Imported {
		for id, obj := range pkgInfo.Defs {
			if id.IsExported() {
				//log.Printf("%s.%v\n", pkgInfo.Pkg.Name(), obj.Name())
				objs[obj.Pkg()] = append(objs[obj.Pkg()], obj)
			}
		}
	}
	return objs
}

func usedObjects(prog *loader.Program) map[*types.Package]Set {
	objs := make(map[*types.Package]Set)
	for _, pkgInfo := range prog.Imported {
		//for _, typeandvalue := range pkgInfo.Types {
		//	log.Printf("types defined %#v\n", typeandvalue)
		//}
		for _, obj := range pkgInfo.Uses {
			if obj.Pkg() == nil {
				continue
			}
			//log.Printf("%s uses %s.%#v.%v\n", pkgInfo.Pkg.Name(), obj.Pkg().Name(), obj.Parent(), id.Name)
			if objs[obj.Pkg()] == nil {
				objs[obj.Pkg()] = make(map[types.Object]bool)
			}
			if obj.Pkg() != pkgInfo.Pkg {
				objs[obj.Pkg()][obj] = true
			}
		}
	}
	return objs
}

func unusedObjects(prog *loader.Program) map[*types.Package][]types.Object {
	exported := exportedObjects(prog)
	used := usedObjects(prog)
	unused := make(map[*types.Package][]types.Object)
	for pkg, objM := range exported {
		for _, obj := range objM {
			if !used[pkg][obj] {
				unused[pkg] = append(unused[pkg], obj)
			}
		}
	}
	return unused
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

func Main(ctx *build.Context, pkgName string) ([]string, error) {
	conf := loader.Config{
		Build: ctx,
	}
	conf.Import(pkgName)
	prog, err := conf.Load()
	if err != nil {
		log.Fatal(err)
	}

	unused := unusedObjects(prog)
	var commands []string
	for pkg, objs := range unused {
		for _, obj := range objs {
			cmd := fmt.Sprintf("gorename -from %s -to %s\n", wholePath(obj, pkg, prog), lowerFirst(obj.Name()))
			commands = append(commands, cmd)
		}
	}
	return commands, nil
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
