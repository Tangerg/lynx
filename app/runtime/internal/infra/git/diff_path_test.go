package git

import "testing"

func TestParseUnifiedDiffRecoversBinaryFilePath(t *testing.T) {
	patch := []byte("diff --git a/image.bin b/image.bin\n" +
		"Binary files a/image.bin and b/image.bin differ\n")

	files, truncated, err := parseUnifiedDiff(patch, 5, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || truncated || !files[0].Binary || files[0].Path != "image.bin" {
		t.Fatalf("binary diff = %+v, truncated=%v; want image.bin", files, truncated)
	}
}

func TestParseUnifiedDiffDecodesGitQuotedPath(t *testing.T) {
	patch := []byte("diff --git \"a/odd\\tname.txt\" \"b/odd\\tname.txt\"\n" +
		"--- \"a/odd\\tname.txt\"\n" +
		"+++ \"b/odd\\tname.txt\"\n" +
		"@@ -1 +1 @@\n" +
		"-before\n" +
		"+after\n")

	files, truncated, err := parseUnifiedDiff(patch, 5, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || truncated || files[0].Path != "odd\tname.txt" {
		t.Fatalf("quoted diff = %+v, truncated=%v; want decoded path", files, truncated)
	}
}
