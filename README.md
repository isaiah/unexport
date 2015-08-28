gounexport
==========

Find unnecessarily exported identifiers in a package and help unexport them.

Install
------------

```shell
go get -u github.com/isaiah/unexport
```

Usage
-----

```
# use -dryrun to show the changes, by default it will try to unexport the
# identifier by lowercase the first character
unexport -dryrun cmd/compile/internal/gc

# under your desired project in GOPATH
unexport
# or specify targeting pakcage
unexport cmd/compile/internal/gc
```

Note it doesn't go recursively into the package, you'll have to  check
each package separately.

Run `unexport -help` to check the other options

Credits
-------

This is the solution for the 5th Go challenge
