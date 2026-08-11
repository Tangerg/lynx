// Package sideload discovers and adapts executable plugins installed outside
// the CLI binary.
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
	manifestName          = "lyra-plugin.json"
	manifestSchemaVersion = 1
	maxManifestBytes      = 1 << 20
	defaultCommandTimeout = 10 * time.Second
	maxCommandTimeout     = 60 * time.Second
	maxManifestCommands   = 128
	maxCommandAliases     = 16
	maxCommandNameBytes   = 64
	maxCommandTitleBytes  = 256
)

type DirectorySource struct {
	directories []string
}

func New(directories []string) DirectorySource {
	return DirectorySource{directories: slices.Clone(directories)}
}

func (DirectorySource) ID() string { return "sideload" }

func (s DirectorySource) Discover(ctx context.Context) (extensions.SourceResult, error) {
	discovery := directoryDiscovery{
		scannedRoots: make(map[string]struct{}, len(s.directories)),
		seenPlugins:  make(map[string]struct{}),
	}
	for _, configured := range s.directories {
		if err := context.Cause(ctx); err != nil {
			return extensions.SourceResult{}, err
		}
		discovery.scanRoot(configured)
	}
	return discovery.sourceResult, nil
}

type directoryDiscovery struct {
	sourceResult extensions.SourceResult
	scannedRoots map[string]struct{}
	seenPlugins  map[string]struct{}
}

func (d *directoryDiscovery) scanRoot(configured string) {
	root, err := canonicalDirectory(configured)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		d.sourceResult.Issues = append(d.sourceResult.Issues, fmt.Errorf("resolve plugin directory %q: %w", configured, err))
		return
	}
	rootKey := pathKey(root)
	if _, scanned := d.scannedRoots[rootKey]; scanned {
		return
	}
	d.scannedRoots[rootKey] = struct{}{}
	entries, err := os.ReadDir(root)
	if err != nil {
		d.sourceResult.Issues = append(d.sourceResult.Issues, fmt.Errorf("read plugin directory %q: %w", root, err))
		return
	}
	d.discover(root)
	for _, entry := range entries {
		d.discoverChild(root, entry)
	}
}

func (d *directoryDiscovery) discoverChild(root string, entry os.DirEntry) {
	if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
		return
	}
	path := filepath.Join(root, entry.Name())
	directory, err := canonicalDirectory(path)
	if err != nil {
		d.sourceResult.Issues = append(d.sourceResult.Issues, fmt.Errorf("resolve plugin directory %q: %w", path, err))
		return
	}
	d.discover(directory)
}

func (d *directoryDiscovery) discover(directory string) {
	discoverDirectory(&d.sourceResult, d.seenPlugins, directory)
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
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("not a directory")
	}
	return filepath.Clean(canonical), nil
}

func pathKey(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

type pluginManifest struct {
	SchemaVersion int                   `json:"schemaVersion"`
	ID            string                `json:"id"`
	Version       string                `json:"version"`
	APIVersion    int                   `json:"apiVersion"`
	Requires      []string              `json:"requires"`
	Capabilities  []string              `json:"capabilities"`
	Entry         string                `json:"entry"`
	Contributes   manifestContributions `json:"contributes"`
}

type manifestContributions struct {
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
	if !info.Mode().IsRegular() || info.Size() > maxManifestBytes {
		return extensions.Plugin{}, false, fmt.Errorf("plugin manifest %q must be a regular file no larger than %d bytes", path, maxManifestBytes)
	}
	var declared pluginManifest
	decoder := json.NewDecoder(io.LimitReader(file, maxManifestBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&declared); err != nil {
		return extensions.Plugin{}, false, fmt.Errorf("decode plugin manifest %q: %w", path, err)
	}
	if err := rejectTrailingJSON(decoder); err != nil {
		return extensions.Plugin{}, false, fmt.Errorf("decode plugin manifest %q: %w", path, err)
	}
	plugin, err := compilePlugin(directory, declared)
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

func compilePlugin(directory string, declared pluginManifest) (extensions.Plugin, error) {
	plugin, err := compilePluginMetadata(declared)
	if err != nil {
		return extensions.Plugin{}, err
	}
	executable, workingDirectory, err := resolveEntry(directory, declared.Entry)
	if err != nil {
		return extensions.Plugin{}, err
	}
	commands, err := compileCommands(declared.ID, executable, workingDirectory, declared.Contributes.Commands)
	if err != nil {
		return extensions.Plugin{}, err
	}
	plugin.Setup = contributeCommands(commands)
	return plugin, nil
}

func compilePluginMetadata(declared pluginManifest) (extensions.Plugin, error) {
	if declared.SchemaVersion != manifestSchemaVersion {
		return extensions.Plugin{}, fmt.Errorf("schemaVersion is %d, want %d", declared.SchemaVersion, manifestSchemaVersion)
	}
	if declared.ID == "terminal" || strings.HasPrefix(declared.ID, "terminal.") {
		return extensions.Plugin{}, fmt.Errorf("plugin id %q uses the reserved terminal namespace", declared.ID)
	}
	if len(declared.Contributes.Commands) == 0 {
		return extensions.Plugin{}, errors.New("contributes.commands must contain at least one command")
	}
	capabilities := make([]extensions.Capability, len(declared.Capabilities))
	for i, capability := range declared.Capabilities {
		capabilities[i] = extensions.Capability(capability)
	}
	plugin := extensions.Plugin{
		ID: declared.ID, Version: declared.Version, APIVersion: declared.APIVersion,
		Requires: slices.Clone(declared.Requires), Capabilities: capabilities,
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
	if len(manifests) > maxManifestCommands {
		return nil, fmt.Errorf("contributes.commands exceeds %d entries", maxManifestCommands)
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
	commandExecutor := executableCommand{
		pluginID: pluginID, command: name, executable: executable,
		directory: directory, timeout: timeout,
	}
	return terminal.SlashCommand{
		Descriptor: terminal.CommandDescriptor{
			Name: name, Title: title, Aliases: aliases, Takes: declared.Takes,
		},
		Execute: commandExecutor.Execute,
	}, nil
}

func validCommandSpelling(value string) bool {
	return value != "" && len(value) <= maxCommandNameBytes && !strings.ContainsAny(value, " /\t\n")
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
	if len(title) > maxCommandTitleBytes {
		return "", fmt.Errorf("command %q title exceeds %d bytes", name, maxCommandTitleBytes)
	}
	return title, nil
}

func compileAliases(name string, declared []string, seen map[string]struct{}) ([]string, error) {
	if len(declared) > maxCommandAliases {
		return nil, fmt.Errorf("command %q exceeds %d aliases", name, maxCommandAliases)
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
	timeout := defaultCommandTimeout
	if seconds != 0 {
		timeout = time.Duration(seconds) * time.Second
	}
	if timeout <= 0 || timeout > maxCommandTimeout {
		return 0, fmt.Errorf("command %q timeout must be between 1 and %.0f seconds", name, maxCommandTimeout.Seconds())
	}
	return timeout, nil
}

var _ extensions.Source = DirectorySource{}
