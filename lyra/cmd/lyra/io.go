package main

import (
	"io"
	"os"
)

// stdout / stderr are indirected through small accessors so tests
// can swap them without touching every subcommand's signature. The
// default values are the real OS streams; tests assign overrides
// from package-level init or via separate testing helpers.
//
// Kept as variables (not consts) so a test can shim them.
var (
	outStream io.Writer = os.Stdout
	errStream io.Writer = os.Stderr
)

func stdout() io.Writer { return outStream }
func stderr() io.Writer { return errStream }
