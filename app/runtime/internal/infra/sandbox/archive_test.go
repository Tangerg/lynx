package sandbox

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveTreeIsDeterministic(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := archiveTree(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := archiveTree(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("unchanged tree produced different snapshots")
	}
}

func TestExtractArchiveRejectsTraversalAndEscapingSymlink(t *testing.T) {
	for _, test := range []struct {
		name     string
		header   tar.Header
		contents string
	}{
		{name: "parent traversal", header: tar.Header{Name: "../outside", Typeflag: tar.TypeReg, Size: 1, Mode: 0o644}, contents: "x"},
		{name: "absolute path", header: tar.Header{Name: "/outside", Typeflag: tar.TypeReg, Size: 1, Mode: 0o644}, contents: "x"},
		{name: "escaping symlink", header: tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../outside", Mode: 0o777}},
		{name: "device", header: tar.Header{Name: "device", Typeflag: tar.TypeChar, Mode: 0o600}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var body bytes.Buffer
			writer := tar.NewWriter(&body)
			if err := writer.WriteHeader(&test.header); err != nil {
				t.Fatal(err)
			}
			if test.contents != "" {
				if _, err := writer.Write([]byte(test.contents)); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := extractArchive(t.Context(), t.TempDir(), body.Bytes()); err == nil {
				t.Fatal("unsafe archive was accepted")
			}
		})
	}
}

func TestArchiveTreeRejectsEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink("../outside", filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := archiveTree(t.Context(), root); err == nil {
		t.Fatal("escaping source symlink was accepted")
	}
}
