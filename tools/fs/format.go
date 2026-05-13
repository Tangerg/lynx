package fs

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// utf8BOM is the UTF-8 byte-order-mark. Some editors / Windows tools
// write it; the LLM shouldn't see it and shouldn't need to type it.
const utf8BOM = "\ufeff"

// hasUTF8BOM reports whether b starts with the UTF-8 BOM.
func hasUTF8BOM(b []byte) bool {
	return len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF
}

// normalizeText strips UTF-8 BOM and converts CRLF to LF. Returns the
// normalized content as a string plus flags so a downstream Write can
// restore the original format.
func normalizeText(data []byte) (text string, hadBOM, hadCRLF bool) {
	if hasUTF8BOM(data) {
		data = data[3:]
		hadBOM = true
	}
	if bytes.Contains(data, []byte("\r\n")) {
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		hadCRLF = true
	}
	return string(data), hadBOM, hadCRLF
}

// restoreFormat re-applies CRLF and BOM to text if the original file
// had them. The LLM always speaks LF + no-BOM; restoration happens
// here so round-trips don't silently flip Windows line endings.
func restoreFormat(text string, hadBOM, hadCRLF bool) []byte {
	if hadCRLF {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	if hadBOM {
		return append([]byte(utf8BOM), text...)
	}
	return []byte(text)
}

// atomicWriteFile writes data to path through a sibling temp file +
// rename. On POSIX the rename is atomic as long as both paths are on
// the same filesystem — so partial writes never leave a half-written
// file visible to readers.
func atomicWriteFile(path string, data []byte, mode os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".lynx-write-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			os.Remove(tmpPath)
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmpPath, mode); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
