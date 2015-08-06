package unexport

import (
	"fmt"
	"go/build"
	"golang.org/x/tools/go/buildutil"
	"testing"
)

func TestUnusedVar(t *testing.T) {
	for _, test := range []struct {
		ctx  *build.Context
		pkg  string
		want []interface{}
	}{
		// init data
		// unused var
		{ctx: main(`package main; var Unused int = 1`),
			pkg:  "main",
			want: []interface{}{"main.Unused", "unused"},
		},
		// unused const
		{ctx: main(`package main; const Unused int = 1`),
			pkg:  "main",
			want: []interface{}{"main.Unused", "unused"},
		},
		// unused type
		{ctx: main(`package main; type S int`),
			pkg:  "main",
			want: []interface{}{"main.S", "s"},
		},
		// unused type field
		{ctx: main(`package main; type s struct { T int }`),
			pkg:  "main",
			want: []interface{}{"(main.s).T", "t"},
		},
		// unused type method
		{ctx: main(`package main; type s int; func (s) F(){}`),
			pkg:  "main",
			want: []interface{}{"(main.s).F", "f"},
		},
	} {
		// test body
		cmds, err := Main(test.ctx, "main")
		if err != nil {
			t.Fatal(err)
		}
		if cmds[0] != formatCmd(test.want) {
			t.Errorf("expected %s, got %s", formatCmd(test.want), cmds[0])
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

func formatCmd(paths []interface{}) string {
	return fmt.Sprintf("gorename -from %s -to %s\n", paths...)
}
