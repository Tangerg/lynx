package main

import (
	"bytes"
	"errors"
	"testing"
	"unsafe"
)

func TestNativeHostUsesExplicitCancellationAndValidatesPaths(t *testing.T) {
	t.Parallel()

	host, err := newNativeHost(fakeWindow{}, fakePicker{}, &fakeSaver{}, fakeDocuments{})
	if err != nil {
		t.Fatalf("newNativeHost() error = %v", err)
	}
	selection, err := host.ChooseDirectory()
	if err != nil || selection.Type != DirectoryCanceled || selection.Path != "" {
		t.Fatalf("selection = %+v, error = %v", selection, err)
	}
}

func TestNativeHostValidatesInlineImageBeforeNativeSave(t *testing.T) {
	t.Parallel()

	saver := &fakeSaver{}
	host, err := newNativeHost(fakeWindow{}, fakePicker{}, saver, fakeDocuments{})
	if err != nil {
		t.Fatalf("newNativeHost() error = %v", err)
	}
	result, err := host.SaveImage("data:image/png;base64,aGVsbG8=")
	if err != nil || result.Type != ImageSaved || !bytes.Equal(saver.contents, []byte("hello")) {
		t.Fatalf("SaveImage() = %+v, error=%v, contents=%q", result, err, saver.contents)
	}
	if _, err := host.SaveImage("https://example.test/image.png"); err == nil {
		t.Fatal("SaveImage() accepted a remote URL")
	}
}

type fakeWindow struct{}

func (fakeWindow) NativeWindow() unsafe.Pointer { return unsafe.Pointer(nil) }

type fakePicker struct {
	path string
	err  error
}

func (picker fakePicker) ChooseDirectory() (string, error) { return picker.path, picker.err }

type fakeSaver struct {
	saved    bool
	contents []byte
}

func (saver *fakeSaver) SaveImage(_ string, contents []byte) (bool, error) {
	if saver.saved {
		return false, errors.New("called twice")
	}
	saver.saved = true
	saver.contents = bytes.Clone(contents)
	return true, nil
}

type fakeDocuments struct{}

func (fakeDocuments) OpenArtifact(int64) ([]byte, bool, error) { return nil, false, nil }
func (fakeDocuments) SaveExport(string, []byte) (bool, error)  { return false, nil }
