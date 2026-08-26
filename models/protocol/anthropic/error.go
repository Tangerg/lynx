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

func (err *responseError) Error() string           { return err.err.Error() }
func (err *responseError) Unwrap() error           { return err.err }
func (err *responseError) HTTPStatus() int         { return err.status }
func (err *responseError) HTTPHeader() http.Header { return err.header.Clone() }

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
