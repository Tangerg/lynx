package sse

import (
	"context"
	"errors"
	"net/http"
)

func WithSSE(ctx context.Context, response http.ResponseWriter, eventChan chan *Message) error {
	header := response.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")

	flusher, ok := response.(http.Flusher)
	if !ok {
		return errors.New("response is not a http.Flusher")
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case event, ok1 := <-eventChan:
			if !ok1 {
				_, err := response.Write([]byte{'\n', '\n'})
				if err != nil {
					return err
				}
				flusher.Flush()
				return ctx.Err()
			} else {
				marshal, err1 := event.Marshal()
				if err1 != nil {
					return err1
				}
				_, err := response.Write(marshal)
				if err != nil {
					return err
				}
				flusher.Flush()
			}
		}
	}
}
