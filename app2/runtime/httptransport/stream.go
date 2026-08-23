package httptransport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/dispatch"
	"github.com/Tangerg/lynx/app2/runtime/rpcwire"
	"github.com/Tangerg/sse"
)

const (
	streamHeartbeat    = 15 * time.Second
	streamWriteTimeout = 10 * time.Second
)

func (server *Server) serveStream(
	response http.ResponseWriter,
	request *http.Request,
	ack *rpcwire.Response,
	stream dispatch.Stream,
) {
	defer stream.Close()
	encodedAck, err := rpcwire.Encode(ack)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, "response_encoding_failed", "the transport could not encode the stream acknowledgement")
		return
	}
	writer := sse.NewHTTPWriter(response)
	response.WriteHeader(http.StatusOK)
	if err := writeSSE(response, writer, sse.Message{Data: encodedAck}); err != nil {
		return
	}
	for {
		nextCtx, cancel := context.WithTimeout(request.Context(), streamHeartbeat)
		frame, err := stream.Next(nextCtx)
		cancel()
		if errors.Is(err, context.DeadlineExceeded) && request.Context().Err() == nil {
			if err := writeSSEComment(response, writer); err != nil {
				return
			}
			continue
		}
		if errors.Is(err, io.EOF) || request.Context().Err() != nil {
			return
		}
		if err != nil {
			return
		}
		encoded, err := rpcwire.Encode(frame.Message)
		if err != nil {
			return
		}
		if err := writeSSE(response, writer, sse.Message{ID: frame.EventID, Data: encoded}); err != nil {
			return
		}
	}
}

func writeSSE(response http.ResponseWriter, writer *sse.Writer, message sse.Message) error {
	controller := http.NewResponseController(response)
	if err := controller.SetWriteDeadline(time.Now().Add(streamWriteTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	err := writer.Write(message)
	_ = controller.SetWriteDeadline(time.Time{})
	return err
}

func writeSSEComment(response http.ResponseWriter, writer *sse.Writer) error {
	controller := http.NewResponseController(response)
	if err := controller.SetWriteDeadline(time.Now().Add(streamWriteTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	err := writer.Comment("heartbeat")
	_ = controller.SetWriteDeadline(time.Time{})
	return err
}
