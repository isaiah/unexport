package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"github.com/isaiah/unexport"
	"go/build"
	"golang.org/x/tools/go/buildutil"
	"golang.org/x/tools/refactor/rename"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var (
	safe     = flag.Bool("safe", false, "only look for internal packages")
	helpFlag = flag.Bool("help", false, "show usage message")

	errNotGoSourcePath = errors.New("path is not under GOROOT or GOPATH")
)

func init() {
	flag.BoolVar(&rename.DryRun, "dryrun", false, "show the changes, but do not apply them")
	flag.BoolVar(&rename.Verbose, "v", false, "print extra verbose information, this will set gorename to verbose mode")
}

func main() {
	flag.Parse()
	if *helpFlag {
		flag.Usage()
		return
	}
	ctxt := &build.Default
	var path []string
	if len(flag.Args()) == 0 {
		path = getwdPackages(ctxt)
	} else {
		path = flag.Args()
	}
	renames, err := unexport.Main(ctxt, path)
	if err != nil {
		panic(err)
	}

	var runall bool
	// apply the changes
	for from, to := range renames {
		if runall {
			if err := rename.Main(ctxt, "", from, to); err != nil {
				if err != nil {
					panic(err)
				}
			}
			continue
		}

		var s string
		fmt.Printf("unexport %s, y/n/c/A? ", from)
		fmt.Scanf("%s", &s)
		switch s {
		case "y", "Y", "A":
			if err := rename.Main(ctxt, "", from, to); err != nil {
				if err != nil {
					panic(err)
				}
			}
			if s == "A" {
				runall = true
			}
		case "c":
			os.Exit(1)
		default:
			continue
		}
	}
}

func getImportPath(ctxt *build.Context, pathOrFilename string) (string, error) {
	dirSlash := filepath.ToSlash(pathOrFilename)

	// We assume that no source root (GOPATH[i] or GOROOT) contains any other.
	for _, srcdir := range ctxt.SrcDirs() {
		srcdirSlash := filepath.ToSlash(srcdir) + "/"
		if strings.HasPrefix(dirSlash, srcdirSlash) {
			importPath := dirSlash[len(srcdirSlash):len(dirSlash)]
			return importPath, nil
		}
	}
	return "", errNotGoSourcePath
}

func getwdPackages(ctxt *build.Context) (folders []string) {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	if !buildutil.FileExists(&build.Default, wd) {
		flag.Usage()
		os.Exit(1)
	}
	err = filepath.Walk(wd, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && !strings.Contains(path, ".git") {
			importPath, err := getImportPath(ctxt, path)
			if err != nil {
				return err
			}
			folders = append(folders, importPath)
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	return
}

func checkConflicts(ctxt *build.Context, renames map[string]string) <-chan string {
	dryrun := rename.DryRun
	rename.DryRun = true
	stderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	warnings := make(chan string)
	defer func(dryrun bool) {
		r.Close()
		w.Close()
		rename.DryRun = dryrun
		os.Stderr = stderr
	}(dryrun)

	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		warnings <- buf.String()
	}()
	fmt.Println("start checking")
	for from, to := range renames {
		if err := rename.Main(ctxt, "", from, to); err != nil {
			if err != rename.ConflictError {
				fmt.Fprintf(stderr, "unexport: %s\n", err)
				os.Exit(1)
			}
			fmt.Println(from, "conflicts")
		}
	}
	return warnings
}
