package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const runtimeConfigDirectoryEnvironment = "LYRA_RUNTIME_CONFIG_DIR"

// runtimeConfigDirectories returns config sources in precedence order. A user
// config beside the runtime's durable data wins. Inside the Scope worktree, the existing
// app/runtime development config is a fallback so source checkouts retain one
// provider configuration instead of copying credentials into app/cli.
func runtimeConfigDirectories(runtimeDirectory string) ([]string, error) {
	directories := make([]string, 0, 3)
	explicit := false
	if configured := strings.TrimSpace(os.Getenv(runtimeConfigDirectoryEnvironment)); configured != "" {
		if !filepath.IsAbs(configured) {
			return nil, fmt.Errorf("%s must be an absolute path", runtimeConfigDirectoryEnvironment)
		}
		explicit = true
		directories = append(directories, filepath.Clean(configured))
	}
	runtimeDirectory = filepath.Clean(runtimeDirectory)
	if len(directories) == 0 || directories[0] != runtimeDirectory {
		directories = append(directories, runtimeDirectory)
	}
	if explicit {
		return directories, nil
	}

	if development, ok := discoverDevelopmentRuntimeConfigDirectory(); ok && development != runtimeDirectory {
		directories = append(directories, development)
	}
	return directories, nil
}

func discoverDevelopmentRuntimeConfigDirectory() (string, bool) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", false
	}
	return developmentRuntimeConfigDirectory(workingDirectory)
}

func developmentRuntimeConfigDirectory(start string) (string, bool) {
	directory, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		goWorkspace := filepath.Join(directory, "go.work")
		runtimeModule := filepath.Join(directory, "app", "runtime", "go.mod")
		configFile := filepath.Join(directory, "app", "runtime", "config", "config.yaml")
		if regularFile(goWorkspace) && regularFile(runtimeModule) && regularFile(configFile) {
			return filepath.Dir(configFile), true
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", false
		}
		directory = parent
	}
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
