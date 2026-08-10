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
	"github.com/Tangerg/lynx/app/cli/internal/terminal"
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
	discovery := sourceDiscovery{
		scannedRoots: make(map[string]struct{}, len(s.directories)),
		seenPlugins:  make(map[string]struct{}),
	}
	for _, configured := range s.directories {
		if err := context.Cause(ctx); err != nil {
			return extensions.SourceResult{}, err
		}
		discovery.scanRoot(configured)
	}
	return discovery.result, nil
}

type sourceDiscovery struct {
	result       extensions.SourceResult
	scannedRoots map[string]struct{}
	seenPlugins  map[string]struct{}
}

func (d *sourceDiscovery) scanRoot(configured string) {
	root, err := canonicalDirectory(configured)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		d.result.Issues = append(d.result.Issues, fmt.Errorf("resolve plugin directory %q: %w", configured, err))
		return
	}
	rootKey := pathKey(root)
	if _, scanned := d.scannedRoots[rootKey]; scanned {
		return
	}
	d.scannedRoots[rootKey] = struct{}{}
	entries, err := os.ReadDir(root)
	if err != nil {
		d.result.Issues = append(d.result.Issues, fmt.Errorf("read plugin directory %q: %w", root, err))
		return
	}
	d.discover(root)
	for _, entry := range entries {
		d.discoverChild(root, entry)
	}
}

func (d *sourceDiscovery) discoverChild(root string, entry os.DirEntry) {
	if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
		return
	}
	path := filepath.Join(root, entry.Name())
	directory, err := canonicalDirectory(path)
	if err != nil {
		d.result.Issues = append(d.result.Issues, fmt.Errorf("resolve plugin directory %q: %w", path, err))
		return
	}
	d.discover(directory)
}

func (d *sourceDiscovery) discover(directory string) {
	discoverDirectory(&d.result, d.seenPlugins, directory)
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
	plugin, err := compilePluginMetadata(manifest)
	if err != nil {
		return extensions.Plugin{}, err
	}
	executable, workingDirectory, err := resolveEntry(directory, manifest.Entry)
	if err != nil {
		return extensions.Plugin{}, err
	}
	commands, err := compileCommands(manifest.ID, executable, workingDirectory, manifest.Contributes.Commands)
	if err != nil {
		return extensions.Plugin{}, err
	}
	plugin.Setup = contributeCommands(commands)
	return plugin, nil
}

func compilePluginMetadata(manifest manifest) (extensions.Plugin, error) {
	if manifest.SchemaVersion != manifestSchema {
		return extensions.Plugin{}, fmt.Errorf("schemaVersion is %d, want %d", manifest.SchemaVersion, manifestSchema)
	}
	if manifest.ID == "terminal" || strings.HasPrefix(manifest.ID, "terminal.") {
		return extensions.Plugin{}, fmt.Errorf("plugin id %q uses the reserved terminal namespace", manifest.ID)
	}
	if len(manifest.Contributes.Commands) == 0 {
		return extensions.Plugin{}, errors.New("contributes.commands must contain at least one command")
	}
	capabilities := make([]extensions.Capability, len(manifest.Capabilities))
	for i, capability := range manifest.Capabilities {
		capabilities[i] = extensions.Capability(capability)
	}
	plugin := extensions.Plugin{
		ID: manifest.ID, Version: manifest.Version, APIVersion: manifest.APIVersion,
		Requires: slices.Clone(manifest.Requires), Capabilities: capabilities,
		Setup: func(*extensions.Scope) error { return nil },
	}
	if err := extensions.ValidateManifest(plugin); err != nil {
		return extensions.Plugin{}, err
	}
	return plugin, nil
}

func contributeCommands(commands []terminal.SlashCommand) func(*extensions.Scope) error {
	return func(scope *extensions.Scope) error {
		for i, command := range commands {
			if _, err := extensions.Contribute(scope, terminal.SlashCommands, command, extensions.Contribution{Order: i}); err != nil {
				return err
			}
		}
		return nil
	}
}

func resolveEntry(directory, entry string) (string, string, error) {
	entry, expected, err := validateEntryPath(directory, entry)
	if err != nil {
		return "", "", err
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
	if err := validateExecutable(entry, realExecutable); err != nil {
		return "", "", err
	}
	return realExecutable, realDirectory, nil
}

func validateEntryPath(directory, entry string) (string, string, error) {
	entry = strings.TrimSpace(entry)
	if entry == "" || filepath.IsAbs(entry) || strings.Contains(entry, `\`) {
		return "", "", fmt.Errorf("entry %q must be a relative slash-separated path", entry)
	}
	expected := filepath.Clean(filepath.Join(directory, filepath.FromSlash(entry)))
	safe := filepath.Clean(pathologize.Join(directory, entry))
	if safe != expected {
		return "", "", fmt.Errorf("entry %q contains unsafe path segments", entry)
	}
	return entry, expected, nil
}

func validateExecutable(entry, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect entry %q: %w", entry, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("entry %q is not a regular file", entry)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("entry %q is not executable", entry)
	}
	return nil
}

func compileCommands(pluginID, executable, directory string, manifests []commandManifest) ([]terminal.SlashCommand, error) {
	if len(manifests) > maximumCommands {
		return nil, fmt.Errorf("contributes.commands exceeds %d entries", maximumCommands)
	}
	seen := make(map[string]struct{}, len(manifests))
	commands := make([]terminal.SlashCommand, 0, len(manifests))
	for _, declared := range manifests {
		command, err := compileCommand(pluginID, executable, directory, declared, seen)
		if err != nil {
			return nil, err
		}
		commands = append(commands, command)
	}
	return commands, nil
}

func compileCommand(
	pluginID, executable, directory string,
	declared commandManifest,
	seen map[string]struct{},
) (terminal.SlashCommand, error) {
	name := strings.TrimSpace(declared.Name)
	if !validCommandSpelling(name) {
		return terminal.SlashCommand{}, fmt.Errorf("command name %q is invalid", declared.Name)
	}
	if err := claimCommandSpelling(seen, name, fmt.Sprintf("command %q is declared more than once", name)); err != nil {
		return terminal.SlashCommand{}, err
	}
	title, err := commandTitle(name, declared.Title)
	if err != nil {
		return terminal.SlashCommand{}, err
	}
	aliases, err := compileAliases(name, declared.Aliases, seen)
	if err != nil {
		return terminal.SlashCommand{}, err
	}
	timeout, err := commandTimeout(name, declared.TimeoutSeconds)
	if err != nil {
		return terminal.SlashCommand{}, err
	}
	runner := commandRunner{
		pluginID: pluginID, command: name, executable: executable,
		directory: directory, timeout: timeout,
	}
	return terminal.SlashCommand{
		Name: name, Title: title, Aliases: aliases,
		Takes: declared.Takes, Execute: runner.Execute,
	}, nil
}

func validCommandSpelling(value string) bool {
	return value != "" && len(value) <= maximumName && !strings.ContainsAny(value, " /\t\n")
}

func claimCommandSpelling(seen map[string]struct{}, spelling, message string) error {
	if _, duplicate := seen[spelling]; duplicate {
		return errors.New(message)
	}
	seen[spelling] = struct{}{}
	return nil
}

func commandTitle(name, declared string) (string, error) {
	title := strings.TrimSpace(declared)
	if title == "" {
		return "", fmt.Errorf("command %q has no title", name)
	}
	if len(title) > maximumTitle {
		return "", fmt.Errorf("command %q title exceeds %d bytes", name, maximumTitle)
	}
	return title, nil
}

func compileAliases(name string, declared []string, seen map[string]struct{}) ([]string, error) {
	if len(declared) > maximumAliases {
		return nil, fmt.Errorf("command %q exceeds %d aliases", name, maximumAliases)
	}
	aliases := slices.Clone(declared)
	for i, alias := range aliases {
		alias = strings.TrimSpace(alias)
		if !validCommandSpelling(alias) {
			return nil, fmt.Errorf("command %q alias %q is invalid", name, declared[i])
		}
		if err := claimCommandSpelling(seen, alias, fmt.Sprintf("command spelling %q is declared more than once", alias)); err != nil {
			return nil, err
		}
		aliases[i] = alias
	}
	return aliases, nil
}

func commandTimeout(name string, seconds int) (time.Duration, error) {
	timeout := defaultTimeout
	if seconds != 0 {
		timeout = time.Duration(seconds) * time.Second
	}
	if timeout <= 0 || timeout > maximumTimeout {
		return 0, fmt.Errorf("command %q timeout must be between 1 and %.0f seconds", name, maximumTimeout.Seconds())
	}
	return timeout, nil
}

var _ extensions.Source = Source{}
