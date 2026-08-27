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
	type httpError interface {
		error
		HTTPStatus() int
		HTTPHeader() http.Header
	}
	if _, ok := errors.AsType[httpError](err); ok {
		return err
	}
	apiErr, ok := errors.AsType[*anthropicsdk.Error](err)
	if !ok {
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
