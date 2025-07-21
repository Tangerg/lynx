package ast

import (
	"errors"
)

type Checker struct {
	errors []error
}

func (c *Checker) Errors() []error {
	return c.errors
}

func (c *Checker) Error() error {
	return errors.Join(c.errors...)
}

func (c *Checker) Visit(expr Expr) Visitor {
	return nil
}
