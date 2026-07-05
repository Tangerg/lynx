package main

import (
	"fmt"
	"os"
)

func currentDirectory() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("working directory: %w", err)
	}
	return cwd, nil
}
