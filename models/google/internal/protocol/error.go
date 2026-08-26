package protocol

import (
	"errors"
	"iter"
	"net/http"

	"google.golang.org/genai"
)

type responseError struct {
	err    error
	status int
}

func (r *responseError) Error() string           { return r.err.Error() }
func (r *responseError) Unwrap() error           { return r.err }
func (r *responseError) HTTPStatus() int         { return r.status }
func (r *responseError) HTTPHeader() http.Header { return nil }

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
	var apiErr *genai.APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	return &responseError{err: err, status: apiErr.Code}
}

func (a *api) wrapResult[T any](value *T, err error) (*T, error) {
	return value, a.wrapError(err)
}

func (a *api) wrapSequence[T any](sequence iter.Seq2[*T, error]) iter.Seq2[*T, error] {
	return func(yield func(*T, error) bool) {
		for value, err := range sequence {
			if !yield(value, a.wrapError(err)) {
				return
			}
		}
	}
}
