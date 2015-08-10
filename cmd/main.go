package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/isaiah/unexport"
	"go/build"
	"golang.org/x/tools/go/buildutil"
	"log"
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
	for from, to := range cmds {
		var s string
		fmt.Printf("unexport %s, y/n/c/A?", from)
		fmt.Scanf("%s", &s)
		switch s {
		case "y", "Y":
			args := []string{"-from", from, "-to", to}
			if output, err := exec.Command(gorename, args...).CombinedOutput(); err != nil {
				log.Printf("%#v", string(output))
				panic(err)
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
