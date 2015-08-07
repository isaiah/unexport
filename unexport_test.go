package unexport

import (
	"fmt"
	"go/build"
	"golang.org/x/tools/go/buildutil"
	"strings"
	"testing"
)

func TestUsedIdentifiers(t *testing.T) {
	for _, test := range []struct {
		ctx  *build.Context
		pkgs []string
	}{
		{ctx: fakeContext(
			map[string][]string{
				"foo": {`
package foo
type I interface{
F()
}
`},
				"bar": {`
package bar
import "foo"
type s int
func (s) F() {}
var _ foo.I = s(0)
`},
			},
		),
			pkgs: []string{"foo", "bar"},
		},
	} {
		prog, err := loadProgram(test.ctx, test.pkgs)
		if err != nil {
			t.Fatal(err)
		}
		u := &unexporter{packages: prog.Imported}
		used := u.usedObjects()
		if len(used) != 2 {
			t.Errorf("expected 2 used objects, got %d", len(used))
		}
	}
}

func TestUnusedIdentifiers(t *testing.T) {
	for _, test := range []struct {
		ctx  *build.Context
		pkgs []string
		want []interface{}
	}{
		// init data
		// unused var
		{ctx: main(`package main; var Unused int = 1`),
			pkgs: []string{"main"},
			want: []interface{}{"main.Unused", "unused"},
		},
		// unused const
		{ctx: main(`package main; const Unused int = 1`),
			pkgs: []string{"main"},
			want: []interface{}{"main.Unused", "unused"},
		},
		// unused type
		{ctx: main(`package main; type S int`),
			pkgs: []string{"main"},
			want: []interface{}{"main.S", "s"},
		},
		// unused type field
		{ctx: main(`package main; type s struct { T int }`),
			pkgs: []string{"main"},
			want: []interface{}{"(main.s).T", "t"},
		},
		// unused type method
		{ctx: main(`package main; type s int; func (s) F(){}`),
			pkgs: []string{"main"},
			want: []interface{}{"(main.s).F", "f"},
		},
		// unused interface method
		{ctx: main(`package main; type s interface { F() }`),
			pkgs: []string{"main"},
			want: []interface{}{"(main.s).F", "f"},
		},
		// type used by function
		{ctx: fakeContext(map[string][]string{
			"foo": {`
package foo
type S int
type T int
`},
			"bar": {`
package bar
import "foo"

func f(t *foo.T) {}
`},
		}),
			pkgs: []string{"bar", "foo"},
			want: []interface{}{"foo.S", "s"},
		},
		// type used, but field not used
		{ctx: fakeContext(map[string][]string{
			"foo": {`
package foo
type S struct {
F int
}
`},
			"bar": {`
package bar
import "foo"

var _ foo.S = foo.S{}
`},
		}),
			pkgs: []string{"bar", "foo"},
			want: []interface{}{"(foo.S).F", "f"},
		},
		// type used, but field not used
		{ctx: fakeContext(map[string][]string{
			"foo": {`
package foo
type S struct {
F int
}
`},
			"bar": {`
package bar
import "foo"

var _ foo.S = foo.S{}
`},
		}),
			pkgs: []string{"bar", "foo"},
			want: []interface{}{"(foo.S).F", "f"},
		},
		// type embedded, #4
		{ctx: fakeContext(map[string][]string{
			"foo": {`
package foo
type S struct {
f int
}
`},
			"bar": {`
package bar
import "foo"

type x struct {
*foo.S
}
`},
		}),
			pkgs: []string{"bar", "foo"},
			want: []interface{}{"(bar.x).S", "s"},
		},
		// unused interface type
		{ctx: fakeContext(map[string][]string{
			"foo": {`
package foo
type I interface {
}
`},
		}),
			pkgs: []string{"foo"},
			want: []interface{}{"foo.I", "i"},
		},
		// interface satisfied only within package
		{ctx: fakeContext(map[string][]string{
			"foo": {`
package foo
type i interface{
F()
}
type t int
func (t) F() {}
var _ i = t(0)
`},
		}),
			pkgs: []string{"foo"},
			want: []interface{}{[]interface{}{"(foo.t).F", "f"}, []interface{}{"(foo.i).F", "f"}},
		},
		// interface satisfied by struct type
		{ctx: fakeContext(map[string][]string{
			"foo": {`
package foo
type I interface {
F()
}
`},
			"bar": {`
package bar
import "foo"
type t int
func (t) F() {}
var _ foo.I = t(0)
`},
		}),
			pkgs: []string{"foo", "bar"},
		},
		// interface satisfied by interface
		{ctx: fakeContext(map[string][]string{
			"foo": {`
package foo
type I interface {
F()
}
`},
			"bar": {`
package bar
import "foo"
type j interface {
foo.I
G()
}
type t int
func (t) F() {}
var _ foo.I = t(0)
`},
		}),
			pkgs: []string{"foo", "bar"},
			want: []interface{}{"(bar.j).G", "g"},
		},
	} {
		// test body
		cmds, err := Main(test.ctx, test.pkgs)
		if err != nil {
			t.Fatal(err)
		}
		if len(cmds) > 1 {
			var concated string
			for _, c := range cmds {
				concated += c
			}
			for _, w := range test.want {
				want, ok := w.([]interface{})
				if !ok {
					t.Fatal("test.want should be an array")
				}
				if !strings.Contains(concated, formatCmd(want)) {
					t.Errorf("command %s is not returned", formatCmd(want))
				}
			}
		} else {
			if len(test.want) > 0 {
				if len(cmds) == 0 {
					t.Errorf("expected %s, got none", formatCmd(test.want))
				} else if cmds[0] != formatCmd(test.want) {
					t.Errorf("expected %s, got %s", formatCmd(test.want), cmds[0])
				}
			} else {
				if len(cmds) > 0 {
					t.Errorf("expected no renaming, got\n %v", cmds)
				}
			}
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
