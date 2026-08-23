package approvalpolicy

import (
	"regexp"
	"strings"
)

var (
	commandSeparator = regexp.MustCompile(`&&|\|\||[;|&\n]`)
	forkBomb         = regexp.MustCompile(`:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`)
	deviceDestroyer  = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bmkfs(\.\w+)?\b`),
		regexp.MustCompile(`(?i)\bwipefs\b`),
		regexp.MustCompile(`(?i)\bdd\b[^|;&\n]*\bof=/dev/`),
		regexp.MustCompile(`(?i)>\s*/dev/(sd|nvme|hd|disk|vd)`),
	}
)

// CatastrophicShellCommand is a deliberately narrow confirmation override,
// not a sandbox. It catches only high-confidence destructive forms so even
// Yolo mode cannot make an obvious root/home/device wipe silent.
func CatastrophicShellCommand(command string) bool {
	if command == "" {
		return false
	}
	if strings.Contains(command, "--no-preserve-root") || forkBomb.MatchString(command) {
		return true
	}
	for _, pattern := range deviceDestroyer {
		if pattern.MatchString(command) {
			return true
		}
	}
	for _, segment := range commandSeparator.Split(command, -1) {
		if recursiveForceRemoveOfRootOrHome(segment) {
			return true
		}
	}
	return false
}

func recursiveForceRemoveOfRootOrHome(segment string) bool {
	fields := strings.Fields(segment)
	foundRM := false
	recursive := false
	force := false
	targets := make([]string, 0, 1)
	for _, field := range fields {
		switch {
		case field == "rm" || strings.HasSuffix(field, "/rm"):
			foundRM = true
		case field == "sudo" || field == "command" || field == "env":
		case strings.HasPrefix(field, "--"):
			recursive = recursive || field == "--recursive"
			force = force || field == "--force"
		case strings.HasPrefix(field, "-"):
			recursive = recursive || strings.ContainsAny(field, "rR")
			force = force || strings.Contains(field, "f")
		default:
			targets = append(targets, strings.Trim(field, `"'`))
		}
	}
	if !foundRM || !recursive || !force {
		return false
	}
	for _, target := range targets {
		switch target {
		case "/", "/*", "~", "~/", "~/*", "$HOME", "${HOME}", "$HOME/", "$HOME/*", "*", ".", "..":
			return true
		}
	}
	return false
}
