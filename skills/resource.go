package skills

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
)

// ReadResource reads and closes a bundled skill resource from src.
func ReadResource(ctx context.Context, src ResourceSource, name, resource string) ([]byte, error) {
	if isNil(src) {
		return nil, ErrNilSource
	}
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	if err := validateResourcePath(resource); err != nil {
		return nil, err
	}
	operation := fmt.Sprintf("read resource %q/%q", name, resource)
	if err := contextError(ctx, operation); err != nil {
		return nil, err
	}
	file, err := src.OpenResource(ctx, name, resource)
	file, err = checkedResourceFile(ctx, operation, name, resource, file, err)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(file)
	err = errors.Join(err, ctx.Err())
	closeErr := file.Close()
	err = errors.Join(
		resourceIOError("read", name, resource, err),
		resourceIOError("close", name, resource, closeErr),
	)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func checkedResourceFile(
	ctx context.Context,
	operation string,
	name string,
	resource string,
	file fs.File,
	err error,
) (fs.File, error) {
	if ctxErr := contextError(ctx, operation); ctxErr != nil {
		return nil, errors.Join(ctxErr, closeResourceFile(name, resource, file))
	}
	if err != nil {
		return nil, errors.Join(err, closeResourceFile(name, resource, file))
	}
	if isNil(file) {
		return nil, fmt.Errorf("skills: %s: %w", operation, ErrNilResourceFile)
	}
	return file, nil
}

func closeResourceFile(name, resource string, file fs.File) error {
	if isNil(file) {
		return nil
	}
	return resourceIOError("close", name, resource, file.Close())
}

func resourceIOError(operation, name, resource string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("skills: %s resource %q/%q: %w", operation, name, resource, err)
}

func validateResourcePath(resource string) error {
	if resource == "." || !fs.ValidPath(resource) || strings.ContainsRune(resource, '\\') {
		return fmt.Errorf("%w: %q", ErrResourcePath, resource)
	}
	return nil
}
