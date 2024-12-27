package sse

import (
	"net/http"
)

type Reader struct {
	error        error
	currentEvent Message
	response     *http.Response
	decoder      *messageDecoder
}

func NewReader(resp *http.Response) *Reader {
	return &Reader{
		response: resp,
		decoder:  newMessageDecoder(resp.Body),
	}
}

func (r *Reader) Error() error {
	return r.error
}

func (r *Reader) Current() (Message, error) {
	return r.currentEvent, r.error
}

func (r *Reader) Next() bool {
	err := r.decoder.Error()
	if err != nil {
		r.error = err
		return false
	}

	if !r.decoder.Next() {
		return false
	}
	r.currentEvent = r.decoder.Current()

	return true
}

func (r *Reader) LastID() string {
	return r.decoder.Current().ID
}

func (r *Reader) Close() error {
	return r.decoder.Close()
}
