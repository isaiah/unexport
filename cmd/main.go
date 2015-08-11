package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/isaiah/unexport"
	"go/build"
	"golang.org/x/tools/go/buildutil"
	"os"
	"path/filepath"
	"strings"
)

var (
	safe     = flag.Bool("safe", false, "only look for internal packages")
	helpFlag = flag.Bool("help", false, "show usage message")
	runall   = flag.Bool("all", false, "run all renaming")
	dryrun   = flag.Bool("dryrun", false, "show the changes, but do not apply them")
	verbose  = flag.Bool("v", false, "print extra verbose information, this will set gorename to verbose mode")

	errNotGoSourcePath = errors.New("path is not under GOROOT or GOPATH")
)

func main() {
	flag.Parse()
	if *helpFlag {
		flag.Usage()
		return
	}
	ctxt := &build.Default
	var path string
	if len(flag.Args()) == 0 {
		path = getwdPackages(ctxt)
	} else {
		path = flag.Args()[0]
	}
	unexporter, err := unexport.New(ctxt, path)
	if err != nil {
		panic(err)
	}
	renames := unexporter.Identifiers

	if *runall {
		unexporter.UpdateAll()
		os.Exit(0)
	}

	// apply the changes
	for obj, qualifier := range renames {

		var s string
		fmt.Printf("unexport %s, y/n/c/A? ", qualifier)
		fmt.Scanf("%s", &s)
		switch s {
		case "y", "Y":
			unexporter.Update(obj)
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

func getwdPackages(ctxt *build.Context) string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	if !buildutil.FileExists(&build.Default, wd) {
		flag.Usage()
		os.Exit(1)
	}
	importPath, err := getImportPath(ctxt, wd)
	if err != nil {
		panic(err)
	}
	return importPath
}
