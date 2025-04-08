package sse

import (
	"bytes"
	"sync"
)

// messagePool is a global pool of Message objects for reuse.
// Each Message is pre-allocated with a 1KB data buffer to reduce allocations.
var messagePool = sync.Pool{
	New: func() interface{} {
		return &Message{
			Data: make([]byte, 0, 1024),
		}
	},
}

// GetMessage retrieves a Message from the pool.
// The returned Message is reset and ready for use.
func GetMessage() *Message {
	msg := messagePool.Get().(*Message)
	msg.ID = ""
	msg.Event = ""
	msg.Data = msg.Data[:0]
	msg.Retry = 0
	return msg
}

// ReleaseMessage returns a Message to the pool.
// If the Message's data buffer exceeds 1MB, it is reallocated to prevent memory leaks.
func ReleaseMessage(msg *Message) {
	if msg == nil {
		return
	}
	if cap(msg.Data) > 1024*1024 {
		msg.Data = make([]byte, 0, 1024)
	}
	msg.ID = ""
	msg.Event = ""
	msg.Data = msg.Data[:0]
	msg.Retry = 0
	messagePool.Put(msg)
}

// bufferPool is a global pool of bytes.Buffer objects for reuse.
var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// GetBuffer retrieves a bytes.Buffer from the pool.
// The returned buffer is reset and ready for use.
func GetBuffer() *bytes.Buffer {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// ReleaseBuffer returns a bytes.Buffer to the pool.
// If the buffer's capacity exceeds 1MB, it is reallocated to prevent memory leaks.
func ReleaseBuffer(buf *bytes.Buffer) {
	if buf == nil {
		return
	}
	if buf.Cap() > 1024*1024 {
		buf = new(bytes.Buffer)
	}
	buf.Reset()
	bufferPool.Put(buf)
}
