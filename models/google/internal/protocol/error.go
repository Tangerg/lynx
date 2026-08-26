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

func (err *responseError) Error() string           { return err.err.Error() }
func (err *responseError) Unwrap() error           { return err.err }
func (err *responseError) HTTPStatus() int         { return err.status }
func (err *responseError) HTTPHeader() http.Header { return nil }

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
