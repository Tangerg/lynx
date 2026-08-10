package extensions

import (
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

// ValidateManifest checks the metadata the extension host relies on before
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
	if err := validateDependencies(id, plugin.Requires); err != nil {
		return err
	}
	return validateCapabilities(id, plugin.Capabilities)
}

func validateDependencies(pluginID string, dependencies []string) error {
	seen := make(map[string]struct{}, len(dependencies))
	for _, dependency := range dependencies {
		dependency = strings.TrimSpace(dependency)
		switch {
		case !pluginIDPattern.MatchString(dependency):
			return fmt.Errorf("extensions: plugin %q has invalid dependency %q", pluginID, dependency)
		case dependency == pluginID:
			return fmt.Errorf("extensions: plugin %q depends on itself", pluginID)
		}
		if _, duplicate := seen[dependency]; duplicate {
			return fmt.Errorf("extensions: plugin %q repeats dependency %q", pluginID, dependency)
		}
		seen[dependency] = struct{}{}
	}
	return nil
}

func validateCapabilities(pluginID string, capabilities []Capability) error {
	seen := make(map[Capability]struct{}, len(capabilities))
	for _, capability := range capabilities {
		name := string(capability)
		if !pluginIDPattern.MatchString(name) {
			return fmt.Errorf("extensions: plugin %q has invalid capability %q", pluginID, capability)
		}
		if _, duplicate := seen[capability]; duplicate {
			return fmt.Errorf("extensions: plugin %q repeats capability %q", pluginID, capability)
		}
		seen[capability] = struct{}{}
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
