package media_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/pkg/mime"
)

func mustMime(t *testing.T, s string) *mime.MIME {
	t.Helper()
	mt, err := mime.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return mt
}

func TestNewMedia_RequiresInputs(t *testing.T) {
	if _, err := media.NewMedia(nil, []byte{}); err == nil {
		t.Fatal("nil mime must error")
	} else if !strings.Contains(err.Error(), "mimeType") {
		t.Fatalf("error %q must mention mimeType", err.Error())
	}

	if _, err := media.NewMedia(mustMime(t, "text/plain"), nil); err == nil {
		t.Fatal("nil data must error")
	}
}

func TestNewMedia_AllocatesMetadata(t *testing.T) {
	m, err := media.NewMedia(mustMime(t, "text/plain"), "hi")
	if err != nil {
		t.Fatal(err)
	}
	if m.Metadata == nil {
		t.Fatal("Metadata must be allocated, not nil")
	}
}

func TestMedia_DataAsBytes(t *testing.T) {
	m, _ := media.NewMedia(mustMime(t, "application/octet-stream"), []byte("ok"))
	got, err := m.DataAsBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ok" {
		t.Fatalf("got %q", got)
	}
}

func TestMedia_DataAsBytes_TypeMismatch(t *testing.T) {
	m, _ := media.NewMedia(mustMime(t, "text/plain"), "string-not-bytes")
	if _, err := m.DataAsBytes(); err == nil {
		t.Fatal("type mismatch must error")
	}
}

func TestMedia_DataAsString(t *testing.T) {
	m, _ := media.NewMedia(mustMime(t, "text/plain"), "hi")
	got, err := m.DataAsString()
	if err != nil || got != "hi" {
		t.Fatalf("got %q, err %v", got, err)
	}
}

func TestMedia_DataAsString_TypeMismatch(t *testing.T) {
	m, _ := media.NewMedia(mustMime(t, "application/octet-stream"), []byte{1, 2})
	if _, err := m.DataAsString(); err == nil {
		t.Fatal("type mismatch must error")
	}
}
