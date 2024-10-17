package mime

import (
	"strings"
	"testing"
)

// video/mp4
var magicNumber = []byte{0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70, 0x69, 0x73, 0x6F, 0x6D}

var testCases = []string{
	"*",
	"*/*",
	"text/*+xml",
	"audio/*",
	"text/plain",
	"text/html; charset=UTF-8",
	"application/json",
	"application/xml; version=1.0",
	"image/jpeg",
	"image/png; quality=high",
	"audio/mpeg",
	"video/mp4; codecs=\"avc1.42E01E, mp4a.40.2\"",
	"application/pdf",
	"application/zip; compression=deflate",
}

func TestParse(t *testing.T) {
	for _, testCase := range testCases {
		t.Log(testCase)
		m, err := Parse(testCase)
		if err != nil {
			t.Log(err)
			continue
		}
		t.Log("type", m.Type())
		t.Log("subType", m.SubType())
		t.Log("charset", m.Charset())
		for k, v := range m.Params() {
			t.Log("key", k, "value", v)
		}
		t.Log("string", m.String())
	}
}

func TestDetect(t *testing.T) {
	m, err := Detect(magicNumber)
	if err != nil {
		t.Log(err)
	}
	t.Log("type", m.Type())
	t.Log("subType", m.SubType())
	t.Log("charset", m.Charset())
	for k, v := range m.Params() {
		t.Log("key", k, "value", v)
	}
	t.Log("string", m.String())
}

func TestDetectReader(t *testing.T) {
	m, err := DetectReader(strings.NewReader(string(magicNumber)))
	if err != nil {
		t.Log(err)
	}
	t.Log("type", m.Type())
	t.Log("subType", m.SubType())
	t.Log("charset", m.Charset())
	for k, v := range m.Params() {
		t.Log("key", k, "value", v)
	}
	t.Log("string", m.String())
}

func TestDetectFile(t *testing.T) {
	m, err := DetectFile("./test.json")
	if err != nil {
		t.Log(err)
	}
	t.Log("type", m.Type())
	t.Log("subType", m.SubType())
	t.Log("charset", m.Charset())
	for k, v := range m.Params() {
		t.Log("key", k, "value", v)
	}
	t.Log("string", m.String())
}
