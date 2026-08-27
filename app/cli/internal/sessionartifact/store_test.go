package sessionartifact

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/scope/app/cli/internal/sessiontransfer"
)

func TestStorePublishesWithoutClobberingAndLoadsPortableJSON(t *testing.T) {
	workspace := t.TempDir()
	document, err := sessiontransfer.NewDocument(sessiontransfer.JSON, []byte(`{"version":17}`))
	if err != nil {
		t.Fatal(err)
	}
	store := Store{}
	first, err := store.Publish(workspace, "Portable session", "archive.json", document)
	if err != nil {
		t.Fatal(err)
	}
	canonicalWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if first != filepath.Join(canonicalWorkspace, "archive.json") {
		t.Fatalf("first path = %q", first)
	}
	if writeFileErr := os.WriteFile(first, []byte("different"), 0o600); writeFileErr != nil {
		t.Fatal(writeFileErr)
	}
	second, err := store.Publish(workspace, "Portable session", "archive.json", document)
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("different existing document was overwritten")
	}
	loaded, err := store.Load(workspace, filepath.Base(second))
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded.Bytes()) != `{"version":17}` {
		t.Fatalf("loaded body = %q", loaded.Bytes())
	}
}

func TestStoreRejectsPathsAsExportNames(t *testing.T) {
	document, err := sessiontransfer.NewDocument(sessiontransfer.Markdown, []byte("# Session"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{}).Publish(t.TempDir(), "Session", "../escape.md", document); err == nil {
		t.Fatal("path-shaped export name was accepted")
	}
}
