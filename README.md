gounexport
==========

Find unnecessarily exported identifiers in a package and help unexport them.

Requirement
-----------

Test and developed on Go 1.5

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

How does it work
----------------

First it will analyze usage of each idenfiers of the current package in the
whole scope of workspace (GOPATH & GOROOT), and then use `gorename` to check
conflicts and apply the changes. For performance reasons, (thread safty & caching) the code is adopted
from `x/tools/refactor/rename`.

Credits
-------

This is the solution for the 5th Go challenge
