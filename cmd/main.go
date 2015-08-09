package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/isaiah/unexport"
	"go/build"
	"golang.org/x/tools/go/buildutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	safe               bool
	errNotGoSourcePath = errors.New("path is not under GOROOT or GOPATH")
)

func init() {
	flag.BoolVar(&safe, "safe", false, "Safe mode")
}

func main() {
	flag.Parse()
	ctxt := &build.Default
	var path []string
	if len(flag.Args()) == 0 {
		path = getwdPackages(ctxt)
	} else {
		path = flag.Args()
	}
	cmds, err := unexport.Main(ctxt, path)
	if err != nil {
		panic(err)
	}
	gorename, err := exec.LookPath("gorename")
	if err != nil {
		fmt.Println("gorename not found in PATH")
	}
	for _, cmd := range cmds {
		var s string
		fmt.Println("gorenmae "+cmd, "y/n/c/A?")
		fmt.Scanf("%s", &s)
		switch s {
		case "y", "Y":
			if err := exec.Command(gorename + " " + cmd).Run(); err != nil {
				panic(err)
			}
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
