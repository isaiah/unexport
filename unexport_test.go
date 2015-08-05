package unexport

import (
	"fmt"
	"go/build"
	"golang.org/x/tools/go/buildutil"
	"golang.org/x/tools/go/loader"
	"log"
	"testing"
)

var (
	prog *loader.Program
	err  error
)

func init() {
	conf := loader.Config{
		SourceImports: true,
		AllowErrors:   false,
	}
	conf.Import("github.com/isaiah/unexport/test_data/b")
	prog, err = conf.Load()
	if err != nil {
		log.Fatal(err)
	}
}

func TestExportedObjects(t *testing.T) {
	objs := exportedObjects(prog)
	if len(objs) != 2 {
		t.Errorf("expected 2 packages, got %d", len(objs))
	}
}

func TestUsedObjects(t *testing.T) {
	objs := usedObjects(prog)
	if len(objs) != 3 {
		t.Errorf("expected 3 packages, got %d", len(objs))
	}
}

func TestUnusedObjects(t *testing.T) {
	unused := unusedObjects(prog)
	for pkg, objs := range unused {
		for _, obj := range objs {
			//log.Printf("%v.%v from %v is not used\n", obj.Type(), obj.Id(), pkg.Name())
			log.Printf("gorename -from %s -to %s\n", wholePath(obj, pkg, prog), lowerFirst(obj.Name()))
		}
	}
}

// ---------------------------------------------------------------------

// Simplifying wrapper around buildutil.FakeContext for packages whose
// filenames are sequentially numbered (%d.go).  pkgs maps a package
// import path to its list of file contents.
func fakeContext(pkgs map[string][]string) *build.Context {
	pkgs2 := make(map[string]map[string]string)
	for path, files := range pkgs {
		filemap := make(map[string]string)
		for i, contents := range files {
			filemap[fmt.Sprintf("%d.go", i)] = contents
		}
		pkgs2[path] = filemap
	}
	return buildutil.FakeContext(pkgs2)
}

// helper for single-file main packages with no imports.
func main(content string) *build.Context {
	return fakeContext(map[string][]string{"main": {content}})
}
