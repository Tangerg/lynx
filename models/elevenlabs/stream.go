package elevenlabs

import (
	"io"
	"iter"
)

func readAudioChunks(reader io.Reader) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		for {
			buffer := make([]byte, 16*1024)
			read, err := reader.Read(buffer)
			eof := err == io.EOF
			if eof {
				err = nil
			}
			if read > 0 || err != nil {
				if !yield(buffer[:read], err) {
					return
				}
			}
			if eof || err != nil {
				return
			}
		}
	}
}
