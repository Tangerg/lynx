package repoarch

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRepositoryUsesOnlyCanonicalScopeIdentity(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	command := exec.Command("git", "-C", root, "ls-files", "-z")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("list tracked repository files: %v", err)
	}

	retiredBrand := "ly" + "nx"
	retiredWord := regexp.MustCompile(`(?i)\b` + retiredBrand + `\b`)
	for _, relative := range strings.Split(string(output), "\x00") {
		if relative == "" {
			continue
		}
		if strings.Contains(strings.ToLower(filepath.ToSlash(relative)), retiredBrand) {
			t.Errorf("tracked path %q uses the retired repository identity", relative)
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Errorf("read tracked file %q: %v", relative, err)
			continue
		}
		if utf8.Valid(data) && retiredWord.Match(data) {
			t.Errorf("tracked file %q uses the retired repository identity", relative)
		}
	}
}
