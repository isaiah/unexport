package packa

import (
	"fmt"
)

var (
	UnusedVar = 1
	UsedVar   = 2
)

const (
	UsedConst   = 3
	UnusedConst = 4
	unusedConst = 5
)

type C interface {
	Count() int
}

type D interface {
	Dump() string
}

type A struct {
	X int
}

func NewA(i int) A {
	return A{i}
}

func (a *A) String() string {
	return fmt.Sprintf("a is %d", a.X)
}

func (a *A) Count() int {
	return a.X
}
