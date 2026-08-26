package anthropic

import (
	"errors"
	"net/http"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

type responseError struct {
	err    error
	status int
	header http.Header
}

func (r *responseError) Error() string           { return r.err.Error() }
func (r *responseError) Unwrap() error           { return r.err }
func (r *responseError) HTTPStatus() int         { return r.status }
func (r *responseError) HTTPHeader() http.Header { return r.header.Clone() }

func (*api) wrapError(err error) error {
	if err == nil {
		return nil
	}
	var responseErr interface {
		HTTPStatus() int
		HTTPHeader() http.Header
	}
	if errors.As(err, &responseErr) {
		return err
	}
	var apiErr *anthropicsdk.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	var header http.Header
	if apiErr.Response != nil {
		header = apiErr.Response.Header.Clone()
	}
	return &responseError{err: err, status: apiErr.StatusCode, header: header}
}

func (a *api) wrapResult[T any](value *T, err error) (*T, error) {
	return value, a.wrapError(err)
}
