package unexport

import (
	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/types"
	"log"
	"testing"
)

var (
	prog *loader.Program
	err  error
)

func init() {
	var conf loader.Config
	conf.Import("github.com/isaiah/unexport/test_data/a")
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
	if len(objs) != 2 {
		t.Errorf("expected 2 packages, got %d", len(objs))
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
