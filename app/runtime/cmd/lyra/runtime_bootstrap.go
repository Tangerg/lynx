package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tangerg/lynx/app/runtime/internal/bootstrap"
	"github.com/Tangerg/lynx/app/runtime/internal/config"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// runtimePaths is the executable's single snapshot of host paths. User Home,
// default workspace, durable data, and launch directory are distinct facts even
// when defaults make some of them descendants or aliases; inner runtime rings
// consume these values and never rediscover them.
type runtimePaths struct {
	userHome             string
	defaultWorkspacePath string
	dataDirectory        string
	launchDirectory      string
}

func resolveRuntimePaths() (runtimePaths, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return runtimePaths{}, fmt.Errorf("runtime: locate user home: %w", err)
	}
	if userHome == "" {
		return runtimePaths{}, errors.New("runtime: user home is unavailable")
	}
	if !filepath.IsAbs(userHome) {
		return runtimePaths{}, errors.New("runtime: user home must be absolute")
	}
	userHome = filepath.Clean(userHome)
	launchDirectory, err := os.Getwd()
	if err != nil {
		return runtimePaths{}, fmt.Errorf("runtime: locate launch directory: %w", err)
	}
	if !filepath.IsAbs(launchDirectory) {
		return runtimePaths{}, errors.New("runtime: launch directory must be absolute")
	}
	dataDirectory := os.Getenv("LYRA_HOME")
	if dataDirectory == "" {
		dataDirectory = filepath.Join(userHome, ".lyra")
	} else if !filepath.IsAbs(dataDirectory) {
		return runtimePaths{}, errors.New("runtime: LYRA_HOME must be an absolute path")
	}
	return runtimePaths{
		userHome:             userHome,
		defaultWorkspacePath: userHome,
		dataDirectory:        filepath.Clean(dataDirectory),
		launchDirectory:      filepath.Clean(launchDirectory),
	}, nil
}

func bootstrapRuntime(ctx context.Context) (_ *bootstrap.Instance, _ config.Settings, _ runtimePaths, err error) {
	paths, err := resolveRuntimePaths()
	if err != nil {
		return nil, config.Settings{}, runtimePaths{}, err
	}
	instance, settings, err := bootstrapRuntimeWithBuildID(ctx, paths, bootstrap.ExecutableBuildID)
	return instance, settings, paths, err
}

func bootstrapRuntimeWithBuildID(
	ctx context.Context,
	paths runtimePaths,
	buildIdentity func() (string, error),
) (*bootstrap.Instance, config.Settings, error) {
	buildID, err := buildIdentity()
	if err != nil {
		return nil, config.Settings{}, err
	}
	return bootstrap.OpenInstance(ctx, bootstrap.InstanceConfig{
		UserHome:             paths.userHome,
		DefaultWorkspacePath: paths.defaultWorkspacePath,
		DataDirectory:        paths.dataDirectory,
		ConfigDirectories: []string{
			filepath.Join(paths.launchDirectory, "config"),
			paths.dataDirectory,
		},
		BuildID: buildID,
		ServerInfo: protocol.ServerInfo{
			Name:    "runtime",
			Version: resolvedVersion(),
		},
	})
}
