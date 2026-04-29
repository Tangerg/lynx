package bufio

import (
	"bufio"
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestDropCR(t *testing.T) {
	tests := []struct {
		in, want []byte
	}{
		{nil, nil},
		{[]byte{}, []byte{}},
		{[]byte("hello\r"), []byte("hello")},
		{[]byte("hello"), []byte("hello")},
		{[]byte("hello\n"), []byte("hello\n")},
		{[]byte("\r"), []byte{}},
		{[]byte("hel\rlo"), []byte("hel\rlo")},
	}
	for _, tt := range tests {
		got := dropCR(tt.in)
		if !bytes.Equal(got, tt.want) {
			t.Errorf("dropCR(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestScanLinesAllFormats_OneShot(t *testing.T) {
	tests := []struct {
		name        string
		data        string
		atEOF       bool
		wantAdvance int
		wantToken   string
		wantNoToken bool
	}{
		{"empty at eof", "", true, 0, "", true},
		{"empty not eof needs more", "", false, 0, "", true},
		{"LF", "hello\n", false, 6, "hello", false},
		{"empty LF", "\n", false, 1, "", false},
		{"CR", "hello\r", false, 6, "hello", false},
		{"CRLF", "hello\r\n", false, 7, "hello", false},
		{"LF before CR", "hello\nworld\r", false, 6, "hello", false},
		{"CR before LF non-paired", "hello\rworld\n", false, 6, "hello", false},
		{"no terminator at eof", "tail", true, 4, "tail", false},
		{"no terminator not eof", "tail", false, 0, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			advance, token, err := ScanLinesAllFormats([]byte(tt.data), tt.atEOF)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if advance != tt.wantAdvance {
				t.Errorf("advance = %d, want %d", advance, tt.wantAdvance)
			}
			if tt.wantNoToken {
				if token != nil {
					t.Errorf("token = %q, want nil", token)
				}
				return
			}
			if string(token) != tt.wantToken {
				t.Errorf("token = %q, want %q", token, tt.wantToken)
			}
		})
	}
}

func scanAll(t *testing.T, in string) []string {
	t.Helper()
	sc := bufio.NewScanner(strings.NewReader(in))
	sc.Split(ScanLinesAllFormats)
	var out []string
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan err: %v", err)
	}
	return out
}

func TestScanLinesAllFormats_Scanner(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"unix", "a\nb\nc\n", []string{"a", "b", "c"}},
		{"windows", "a\r\nb\r\nc\r\n", []string{"a", "b", "c"}},
		{"old mac", "a\rb\rc\r", []string{"a", "b", "c"}},
		{"mixed", "unix\nwindows\r\nmac\r", []string{"unix", "windows", "mac"}},
		{"trailing no eol", "a\nb\nc", []string{"a", "b", "c"}},
		{"empty lines", "\n\n\n", []string{"", "", ""}},
		{"empty input", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scanAll(t, tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func BenchmarkScanLinesAllFormats(b *testing.B) {
	in := strings.Repeat("line\r\n", 1000)
	for b.Loop() {
		sc := bufio.NewScanner(strings.NewReader(in))
		sc.Split(ScanLinesAllFormats)
		for sc.Scan() {
		}
	}
}
