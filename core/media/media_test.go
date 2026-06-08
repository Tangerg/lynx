package media_test

import (
	"bytes"
	"encoding/json"
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

func TestMedia_JSONRoundTrip_Bytes(t *testing.T) {
	want := []byte{0x01, 0x02, 0x03, 0xff}
	src, _ := media.NewMedia(mustMime(t, "image/png"), want)
	src.ID = "m1"
	src.Name = "tiny.png"

	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(data), `"data_encoding":"bytes"`) {
		t.Fatalf("missing data_encoding discriminator: %s", data)
	}

	var got media.Media
	err = json.Unmarshal(data, &got)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	var gotBytes []byte
	gotBytes, err = got.DataAsBytes()
	if err != nil {
		t.Fatalf("DataAsBytes after round-trip: %v", err)
	}
	if !bytes.Equal(gotBytes, want) {
		t.Fatalf("Data = %v, want %v", gotBytes, want)
	}
	if got.ID != "m1" || got.Name != "tiny.png" {
		t.Fatalf("ID/Name lost in round-trip: id=%q name=%q", got.ID, got.Name)
	}
}

func TestMedia_JSONRoundTrip_String(t *testing.T) {
	want := "https://example.com/a.png"
	src, _ := media.NewMedia(mustMime(t, "image/png"), want)

	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(data), `"data_encoding":"text"`) {
		t.Fatalf("missing data_encoding discriminator: %s", data)
	}

	var got media.Media
	err = json.Unmarshal(data, &got)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	var gotStr string
	gotStr, err = got.DataAsString()
	if err != nil {
		t.Fatalf("DataAsString after round-trip: %v", err)
	}
	if gotStr != want {
		t.Fatalf("Data = %q, want %q", gotStr, want)
	}
}

func TestMedia_MarshalJSON_RejectsUnsupportedData(t *testing.T) {
	m := &media.Media{
		MimeType: mustMime(t, "application/json"),
		Data:     struct{ X int }{X: 1},
	}
	if _, err := json.Marshal(m); err == nil {
		t.Fatal("MarshalJSON must reject non-string/non-bytes Data")
	}
}
