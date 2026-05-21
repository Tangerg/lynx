package main

import (
	"flag"
	"io"
	"os"
)

// outStream / errStream / inStream are the indirected IO streams
// every subcommand uses instead of os.Stdout / os.Stderr / os.Stdin
// directly. Tests assign in-memory replacements at TestMain time so
// captured output can be asserted on.
//
// Kept as plain vars (not consts) so a test can shim them.
var (
	outStream io.Writer = os.Stdout
	errStream io.Writer = os.Stderr
	inStream  io.Reader = os.Stdin
)

func stdout() io.Writer { return outStream }
func stderr() io.Writer { return errStream }
func stdin() io.Reader  { return inStream }

// newSubFlagSet is the boilerplate-busting constructor every
// subcommand uses to build its [flag.FlagSet]. Three properties
// are uniform across the CLI and live here so individual
// subcommands don't repeat them:
//
//   - ContinueOnError (we surface usage + return non-zero, not
//     os.Exit out from under the dispatcher)
//   - output directed at stderr() (so tests capture it)
//   - the prefix used in flag's auto-generated error messages
//     matches the subcommand name the user typed
func newSubFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr())
	return fs
}
