package repoarch

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type moduleChecksum struct {
	hash   string
	source string
}

func TestReleaseEntryDerivesModulesAndKeepsTagsImmutable(t *testing.T) {
	t.Parallel()
	path := filepath.Join(repositoryRoot(t), "scripts", "release.sh")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Error("scripts/release.sh must be executable")
	}
	if output, syntaxErr := exec.Command("bash", "-n", path).CombinedOutput(); syntaxErr != nil {
		t.Fatalf("scripts/release.sh syntax: %v\n%s", syntaxErr, output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, required := range []string{
		"go work edit -json",
		"mod edit -json",
		"depth[path] + 0",
		"release plan has invalid layer",
		"release plan has no modules in layer",
		"go clean -modcache",
		"git tag -a",
		"mod download -json",
		"refs/tags/$release_tag:refs/tags/$release_tag",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("scripts/release.sh no longer contains required release boundary %q", required)
		}
	}
	for _, forbidden := range []string{"tag -d", "--force", "push -f", "push --tags"} {
		if strings.Contains(script, forbidden) {
			t.Errorf("scripts/release.sh contains mutable or aggregate tag operation %q", forbidden)
		}
	}
}

func TestInternalModuleChecksumsAreConsistent(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	checksums := make(map[string]moduleChecksum)
	for _, module := range discoverModules(t, root) {
		path := filepath.Join(root, filepath.FromSlash(module.dir), "go.sum")
		file, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatal(err)
		}
		scanModuleChecksums(t, file, path, checksums)
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func scanModuleChecksums(
	t *testing.T,
	file *os.File,
	path string,
	checksums map[string]moduleChecksum,
) {
	t.Helper()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 || !isRepositoryImport(fields[0]) {
			continue
		}
		identity := fields[0] + " " + fields[1]
		previous, present := checksums[identity]
		if present && previous.hash != fields[2] {
			t.Errorf(
				"%s has checksum %s in %s and %s in %s",
				identity, previous.hash, previous.source, fields[2], path,
			)
			continue
		}
		checksums[identity] = moduleChecksum{hash: fields[2], source: path}
	}
	if err := scanner.Err(); err != nil {
		t.Errorf("scan %s: %v", path, err)
	}
}
