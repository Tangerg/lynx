package localruntime_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/localruntime"
)

func TestOpenTokenCreatesDurableCanonicalCredential(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "local-token")
	token, err := localruntime.OpenToken(path)
	if err != nil {
		t.Fatalf("OpenToken() error = %v", err)
	}
	if token.Path() != path {
		t.Fatalf("Path() = %q, want %q", token.Path(), path)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token.Value())
	if err != nil || len(decoded) != 32 {
		t.Fatalf("Value() is not one canonical 32-byte token: bytes=%d error=%v", len(decoded), err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat() error = %v", err)
	}
	if info.Mode().Perm() != 0o600 || info.Size() != int64(len(token.Value())) {
		t.Fatalf("token file = mode %04o size %d", info.Mode().Perm(), info.Size())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != token.Value() {
		t.Fatal("published token differs from returned credential")
	}
}

func TestOpenTokenSurvivesRuntimeReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local-token")
	predecessor, err := localruntime.OpenToken(path)
	if err != nil {
		t.Fatalf("open predecessor: %v", err)
	}
	successor, err := localruntime.OpenToken(path)
	if err != nil {
		t.Fatalf("open successor: %v", err)
	}
	if successor.Value() != predecessor.Value() {
		t.Fatal("successor replaced the durable credential")
	}
}

func TestOpenTokenDoesNotReplaceInvalidCredential(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local-token")
	want := []byte("owned-but-invalid")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := localruntime.OpenToken(path); !errors.Is(err, localruntime.ErrInvalidToken) {
		t.Fatalf("OpenToken() error = %v, want ErrInvalidToken", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("invalid existing credential changed to %q", got)
	}
}

func TestConcurrentOpenTokenCallersConverge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local-token")
	const callers = 16
	values := make(chan string, callers)
	errs := make(chan error, callers)
	var group sync.WaitGroup
	for range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			token, err := localruntime.OpenToken(path)
			if err != nil {
				errs <- err
				return
			}
			values <- token.Value()
		}()
	}
	group.Wait()
	close(values)
	close(errs)
	for err := range errs {
		t.Fatalf("OpenToken() error = %v", err)
	}
	var want string
	for value := range values {
		if want == "" {
			want = value
		}
		if value != want {
			t.Fatalf("concurrent values include %q and %q", want, value)
		}
	}
}

func TestReadTokenRejectsInvalidDurableCredentials(t *testing.T) {
	canonical := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	tests := map[string]struct {
		contents []byte
		mode     os.FileMode
		link     bool
	}{
		"arbitrary text":       {contents: []byte("token"), mode: 0o600},
		"whitespace wrapped":   {contents: []byte("\n" + canonical + "\n"), mode: 0o600},
		"oversized":            {contents: bytes.Repeat([]byte("x"), 1<<20), mode: 0o600},
		"public permissions":   {contents: []byte(canonical), mode: 0o644},
		"non-canonical base64": {contents: []byte(canonical[:len(canonical)-1] + "B"), mode: 0o600},
		"symbolic link":        {contents: []byte(canonical), mode: 0o600, link: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "local-token")
			writePath := path
			if test.link {
				writePath = filepath.Join(root, "actual-token")
			}
			if err := os.WriteFile(writePath, test.contents, test.mode); err != nil {
				t.Fatal(err)
			}
			if test.link {
				if err := os.Symlink(writePath, path); err != nil {
					if runtime.GOOS == "windows" {
						t.Skipf("symlink creation is unavailable: %v", err)
					}
					t.Fatal(err)
				}
			}
			if _, err := localruntime.ReadToken(path); !errors.Is(err, localruntime.ErrInvalidToken) {
				t.Fatalf("ReadToken() error = %v, want ErrInvalidToken", err)
			}
		})
	}
}

func TestTokenOperationsRequireAbsolutePath(t *testing.T) {
	for name, operation := range map[string]func(string) (*localruntime.Token, error){
		"open": localruntime.OpenToken,
		"read": localruntime.ReadToken,
	} {
		t.Run(name, func(t *testing.T) {
			for _, path := range []string{"", "relative-token"} {
				if _, err := operation(path); !errors.Is(err, localruntime.ErrInvalidToken) {
					t.Fatalf("operation(%q) error = %v, want ErrInvalidToken", path, err)
				}
			}
		})
	}
}

func TestReadTokenPreservesMissingFileIdentity(t *testing.T) {
	if _, err := localruntime.ReadToken(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadToken() error = %v, want os.ErrNotExist", err)
	}
}
