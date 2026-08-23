package capabilityflow

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

func readBoundedAuthoredFile(file *os.File, info fs.FileInfo) ([]byte, error) {
	if !info.Mode().IsRegular() {
		return nil, errors.New("authored resource is not a regular file")
	}
	if info.Size() > maxAuthoredDocumentBytes {
		return nil, fmt.Errorf("authored resource exceeds %d bytes", maxAuthoredDocumentBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxAuthoredDocumentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxAuthoredDocumentBytes {
		return nil, fmt.Errorf("authored resource exceeds %d bytes", maxAuthoredDocumentBytes)
	}
	return data, nil
}
