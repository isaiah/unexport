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
	safe         = flag.Bool("safe", false, "only look for internal packages")
	dryRun       = flag.Bool("dryrun", false, "show the changes, but do not apply them")
	verbose      = flag.Bool("v", false, "print verbose information, including gorename output")
	extraVerbose = flag.Bool("vv", false, "print extra verbose information, this will set gorename to verbose mode")
	helpFlag     = flag.Bool("help", false, "show usage message")

	errNotGoSourcePath = errors.New("path is not under GOROOT or GOPATH")
)

func init() {
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
			if *dryRun {
				args = append(args, "-dryrun")
			}
			if *extraVerbose {
				args = append(args, "-v")
			}
			output, err := exec.Command(gorename, args...).CombinedOutput()
			if err != nil {
				panic(err)
			}
			if *verbose || *extraVerbose || *dryRun {
				fmt.Println(string(output))
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
