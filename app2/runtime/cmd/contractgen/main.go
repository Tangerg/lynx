package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Tangerg/lynx/app2/runtime/contractgen"
)

func main() {
	out := flag.String("out", "contract", "artifact output directory")
	check := flag.Bool("check", false, "verify checked-in artifacts without rewriting them")
	flag.Parse()
	if err := run(*out, *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(outputDirectory string, check bool) error {
	artifacts, err := contractgen.Artifacts()
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(artifacts))
	for path := range artifacts {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, relative := range paths {
		path := filepath.Join(outputDirectory, filepath.FromSlash(relative))
		if check {
			existing, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("contractgen: read %s: %w", path, err)
			}
			if !bytes.Equal(existing, artifacts[relative]) {
				return fmt.Errorf("contractgen: %s is stale", path)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("contractgen: create directory for %s: %w", path, err)
		}
		if err := os.WriteFile(path, artifacts[relative], 0o644); err != nil {
			return fmt.Errorf("contractgen: write %s: %w", path, err)
		}
	}
	return nil
}
