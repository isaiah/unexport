package packa

import (
	"fmt"
)

type C interface {
	Count() int
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
