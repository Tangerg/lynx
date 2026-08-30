package a2a_test

import (
	"fmt"

	"github.com/Tangerg/scope/a2a"
)

func ExampleNewJSONRPCInterface() {
	transport := a2a.NewJSONRPCInterface("https://agent.example/invoke")

	fmt.Println(transport.URL, transport.ProtocolBinding)
	// Output:
	// https://agent.example/invoke JSONRPC
}
