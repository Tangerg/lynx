package sideload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/spf13/pathologize"

	"github.com/Tangerg/lynx/app/cli/internal/extensions"
	"github.com/Tangerg/lynx/app/cli/internal/ui/session"
)

const (
	manifestName    = "lyra-plugin.json"
	manifestSchema  = 1
	maximumManifest = 1 << 20
	defaultTimeout  = 10 * time.Second
	maximumTimeout  = 60 * time.Second
	maximumCommands = 128
	maximumAliases  = 16
	maximumName     = 64
	maximumTitle    = 256
)

type Source struct {
	directories []string
}

func New(directories []string) Source {
	return Source{directories: slices.Clone(directories)}
}

func (Source) ID() string { return "sideload" }

func (s Source) Discover(ctx context.Context) (extensions.SourceResult, error) {
	var result extensions.SourceResult
	scannedRoots := make(map[string]struct{}, len(s.directories))
	seenPlugins := make(map[string]struct{})
	for _, configured := range s.directories {
		if err := context.Cause(ctx); err != nil {
			return extensions.SourceResult{}, err
		}
		root, err := canonicalDirectory(configured)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			result.Issues = append(result.Issues, fmt.Errorf("resolve plugin directory %q: %w", configured, err))
			continue
		}
		rootKey := pathKey(root)
		if _, scanned := scannedRoots[rootKey]; scanned {
			continue
		}
		scannedRoots[rootKey] = struct{}{}
		entries, err := os.ReadDir(root)
		if err != nil {
			result.Issues = append(result.Issues, fmt.Errorf("read plugin directory %q: %w", root, err))
			continue
		}
		discoverDirectory(&result, seenPlugins, root)
		for _, entry := range entries {
			if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
				continue
			}
			directory, err := canonicalDirectory(filepath.Join(root, entry.Name()))
			if err != nil {
				result.Issues = append(result.Issues, fmt.Errorf("resolve plugin directory %q: %w", filepath.Join(root, entry.Name()), err))
				continue
			}
			discoverDirectory(&result, seenPlugins, directory)
		}
	}
	return result, nil
}

func discoverDirectory(result *extensions.SourceResult, seen map[string]struct{}, directory string) {
	key := pathKey(directory)
	if _, duplicate := seen[key]; duplicate {
		return
	}
	seen[key] = struct{}{}
	plugin, ok, err := readPlugin(directory)
	if err != nil {
		result.Issues = append(result.Issues, err)
		return
	}
	if ok {
		result.Plugins = append(result.Plugins, plugin)
	}
}

func canonicalDirectory(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(real)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory")
	}
	return filepath.Clean(real), nil
}

func pathKey(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

type manifest struct {
	SchemaVersion int           `json:"schemaVersion"`
	ID            string        `json:"id"`
	Version       string        `json:"version"`
	APIVersion    int           `json:"apiVersion"`
	Requires      []string      `json:"requires"`
	Capabilities  []string      `json:"capabilities"`
	Entry         string        `json:"entry"`
	Contributes   contributions `json:"contributes"`
}

type contributions struct {
	Commands []commandManifest `json:"commands"`
}

type commandManifest struct {
	Name           string   `json:"name"`
	Title          string   `json:"title"`
	Aliases        []string `json:"aliases"`
	Takes          bool     `json:"takes"`
	TimeoutSeconds int      `json:"timeoutSeconds"`
}

func readPlugin(directory string) (extensions.Plugin, bool, error) {
	path := filepath.Join(directory, manifestName)
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return extensions.Plugin{}, false, nil
	}
	if err != nil {
		return extensions.Plugin{}, false, fmt.Errorf("open plugin manifest %q: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return extensions.Plugin{}, false, fmt.Errorf("inspect plugin manifest %q: %w", path, err)
	}
	if !info.Mode().IsRegular() || info.Size() > maximumManifest {
		return extensions.Plugin{}, false, fmt.Errorf("plugin manifest %q must be a regular file no larger than %d bytes", path, maximumManifest)
	}
	var manifest manifest
	decoder := json.NewDecoder(io.LimitReader(file, maximumManifest+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return extensions.Plugin{}, false, fmt.Errorf("decode plugin manifest %q: %w", path, err)
	}
	if err := rejectTrailingJSON(decoder); err != nil {
		return extensions.Plugin{}, false, fmt.Errorf("decode plugin manifest %q: %w", path, err)
	}
	plugin, err := compileManifest(directory, manifest)
	if err != nil {
		return extensions.Plugin{}, false, fmt.Errorf("validate plugin manifest %q: %w", path, err)
	}
	return plugin, true, nil
}

func rejectTrailingJSON(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("input contains multiple JSON values")
}

func compileManifest(directory string, manifest manifest) (extensions.Plugin, error) {
	if manifest.SchemaVersion != manifestSchema {
		return extensions.Plugin{}, fmt.Errorf("schemaVersion is %d, want %d", manifest.SchemaVersion, manifestSchema)
	}
	if manifest.ID == "terminal" || strings.HasPrefix(manifest.ID, "terminal.") {
		return extensions.Plugin{}, fmt.Errorf("plugin id %q uses the reserved terminal namespace", manifest.ID)
	}
	if len(manifest.Contributes.Commands) == 0 {
		return extensions.Plugin{}, errors.New("contributes.commands must contain at least one command")
	}
	executable, workingDirectory, err := resolveEntry(directory, manifest.Entry)
	if err != nil {
		return extensions.Plugin{}, err
	}
	commands, err := compileCommands(manifest.ID, executable, workingDirectory, manifest.Contributes.Commands)
	if err != nil {
		return extensions.Plugin{}, err
	}
	capabilities := make([]extensions.Capability, len(manifest.Capabilities))
	for i, capability := range manifest.Capabilities {
		capabilities[i] = extensions.Capability(capability)
	}
	plugin := extensions.Plugin{
		ID: manifest.ID, Version: manifest.Version, APIVersion: manifest.APIVersion,
		Requires: slices.Clone(manifest.Requires), Capabilities: capabilities,
	}
	plugin.Setup = func(scope *extensions.Scope) error {
		for i, command := range commands {
			if _, err := extensions.Contribute(scope, session.SlashCommands, command, extensions.Contribution{Order: i}); err != nil {
				return err
			}
		}
		return nil
	}
	if err := extensions.ValidateManifest(plugin); err != nil {
		return extensions.Plugin{}, err
	}
	return plugin, nil
}

func resolveEntry(directory, entry string) (string, string, error) {
	entry = strings.TrimSpace(entry)
	if entry == "" || filepath.IsAbs(entry) || strings.Contains(entry, `\`) {
		return "", "", fmt.Errorf("entry %q must be a relative slash-separated path", entry)
	}
	expected := filepath.Clean(filepath.Join(directory, filepath.FromSlash(entry)))
	safe := filepath.Clean(pathologize.Join(directory, entry))
	if safe != expected {
		return "", "", fmt.Errorf("entry %q contains unsafe path segments", entry)
	}
	realDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return "", "", fmt.Errorf("resolve plugin directory: %w", err)
	}
	realExecutable, err := filepath.EvalSymlinks(expected)
	if err != nil {
		return "", "", fmt.Errorf("resolve entry %q: %w", entry, err)
	}
	relative, err := filepath.Rel(realDirectory, realExecutable)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("entry %q escapes its plugin directory", entry)
	}
	info, err := os.Stat(realExecutable)
	if err != nil {
		return "", "", fmt.Errorf("inspect entry %q: %w", entry, err)
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("entry %q is not a regular file", entry)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return "", "", fmt.Errorf("entry %q is not executable", entry)
	}
	return realExecutable, realDirectory, nil
}

func compileCommands(pluginID, executable, directory string, manifests []commandManifest) ([]session.SlashCommand, error) {
	if len(manifests) > maximumCommands {
		return nil, fmt.Errorf("contributes.commands exceeds %d entries", maximumCommands)
	}
	seen := make(map[string]struct{}, len(manifests))
	commands := make([]session.SlashCommand, 0, len(manifests))
	for _, declared := range manifests {
		name := strings.TrimSpace(declared.Name)
		if name == "" || len(name) > maximumName || strings.ContainsAny(name, " /\t\n") {
			return nil, fmt.Errorf("command name %q is invalid", declared.Name)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("command %q is declared more than once", name)
		}
		seen[name] = struct{}{}
		title := strings.TrimSpace(declared.Title)
		if title == "" {
			return nil, fmt.Errorf("command %q has no title", name)
		}
		if len(title) > maximumTitle {
			return nil, fmt.Errorf("command %q title exceeds %d bytes", name, maximumTitle)
		}
		aliases := slices.Clone(declared.Aliases)
		if len(aliases) > maximumAliases {
			return nil, fmt.Errorf("command %q exceeds %d aliases", name, maximumAliases)
		}
		for i, alias := range aliases {
			alias = strings.TrimSpace(alias)
			if alias == "" || len(alias) > maximumName || strings.ContainsAny(alias, " /\t\n") {
				return nil, fmt.Errorf("command %q alias %q is invalid", name, declared.Aliases[i])
			}
			if _, duplicate := seen[alias]; duplicate {
				return nil, fmt.Errorf("command spelling %q is declared more than once", alias)
			}
			seen[alias] = struct{}{}
			aliases[i] = alias
		}
		timeout := defaultTimeout
		if declared.TimeoutSeconds != 0 {
			timeout = time.Duration(declared.TimeoutSeconds) * time.Second
		}
		if timeout <= 0 || timeout > maximumTimeout {
			return nil, fmt.Errorf("command %q timeout must be between 1 and %.0f seconds", name, maximumTimeout.Seconds())
		}
		runner := commandRunner{
			pluginID: pluginID, command: name, executable: executable,
			directory: directory, timeout: timeout,
		}
		commands = append(commands, session.SlashCommand{
			Name: name, Title: title, Aliases: aliases,
			Takes: declared.Takes, Execute: runner.Execute,
		})
	}
	return commands, nil
}

var _ extensions.Source = Source{}
