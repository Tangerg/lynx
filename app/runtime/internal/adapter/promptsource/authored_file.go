package promptsource

import (
	"context"
	"fmt"
	"io"
	"os"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
)

func readAuthoredPromptFile(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %q is not a regular file", workspaceapp.ErrInvalidPromptSource, path)
	}
	if err := workspaceapp.ValidateAuthoredPromptDocumentSize(info.Size()); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	document, err := io.ReadAll(io.LimitReader(
		promptContextReader{ctx: ctx, reader: file},
		workspaceapp.MaxAuthoredPromptDocumentBytes+1,
	))
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}
	if err := workspaceapp.ValidateAuthoredPromptDocument(document); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return document, nil
}

type promptContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader promptContextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}
