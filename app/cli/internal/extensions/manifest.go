package extensions

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/mod/semver"
)

const HostAPIVersion = 1

var (
	pluginIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)
)

// ValidateManifest checks the metadata the extension kernel relies on before
// any plugin code runs.
func ValidateManifest(plugin Plugin) error {
	id := strings.TrimSpace(plugin.ID)
	if !pluginIDPattern.MatchString(id) {
		return fmt.Errorf("extensions: plugin id %q must be a lowercase dotted identifier", plugin.ID)
	}
	if !semver.IsValid("v" + plugin.Version) {
		return fmt.Errorf("extensions: plugin %q has invalid semantic version %q", id, plugin.Version)
	}
	if plugin.APIVersion != HostAPIVersion {
		return fmt.Errorf("extensions: plugin %q requires host API %d, host provides %d", id, plugin.APIVersion, HostAPIVersion)
	}
	if plugin.Setup == nil {
		return fmt.Errorf("extensions: plugin %q has no setup", id)
	}
	seenDependencies := make(map[string]struct{}, len(plugin.Requires))
	for _, dependency := range plugin.Requires {
		dependency = strings.TrimSpace(dependency)
		switch {
		case !pluginIDPattern.MatchString(dependency):
			return fmt.Errorf("extensions: plugin %q has invalid dependency %q", id, dependency)
		case dependency == id:
			return fmt.Errorf("extensions: plugin %q depends on itself", id)
		}
		if _, duplicate := seenDependencies[dependency]; duplicate {
			return fmt.Errorf("extensions: plugin %q repeats dependency %q", id, dependency)
		}
		seenDependencies[dependency] = struct{}{}
	}
	seenCapabilities := make(map[Capability]struct{}, len(plugin.Capabilities))
	for _, capability := range plugin.Capabilities {
		name := string(capability)
		if !pluginIDPattern.MatchString(name) {
			return fmt.Errorf("extensions: plugin %q has invalid capability %q", id, capability)
		}
		if _, duplicate := seenCapabilities[capability]; duplicate {
			return fmt.Errorf("extensions: plugin %q repeats capability %q", id, capability)
		}
		seenCapabilities[capability] = struct{}{}
	}
	return nil
}

func clonePlugin(plugin Plugin) Plugin {
	plugin.ID = strings.Clone(plugin.ID)
	plugin.Version = strings.Clone(plugin.Version)
	plugin.Requires = slices.Clone(plugin.Requires)
	plugin.Capabilities = slices.Clone(plugin.Capabilities)
	return plugin
}

var errKernelClosed = errors.New("extensions: kernel is closed")
