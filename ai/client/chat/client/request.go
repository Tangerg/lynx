package client

type ChatClientRequest interface {
	Call() CallResponse
	Stream() StreamResponse
}
