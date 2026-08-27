package dispatch

import (
	"encoding/json"

	"github.com/Tangerg/scope/app/runtime/internal/delivery/transport"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

const requestMetaField = "_meta"

// extractRequestMeta removes the JSON-RPC-only _meta member and returns its
// binding-neutral protocol value. Semantic validation belongs to operation.
func extractRequestMeta(request *transport.Request) (protocol.RequestMeta, *transport.Error) {
	if request == nil || len(request.Params) == 0 {
		return protocol.RequestMeta{}, nil
	}
	var parameters map[string]json.RawMessage
	if err := json.Unmarshal(request.Params, &parameters); err != nil {
		return protocol.RequestMeta{}, nil
	}
	encoded, ok := parameters[requestMetaField]
	if !ok {
		return protocol.RequestMeta{}, nil
	}
	var metadata protocol.RequestMeta
	if err := decodeParams(encoded, &metadata); err != nil {
		return protocol.RequestMeta{}, invalidParams(requestMetaField + ": " + err.Error())
	}
	delete(parameters, requestMetaField)
	if len(parameters) == 0 {
		request.Params = nil
		return metadata, nil
	}
	encodedParameters, err := json.Marshal(parameters)
	if err != nil {
		return protocol.RequestMeta{}, invalidParams(requestMetaField + ": " + err.Error())
	}
	request.Params = encodedParameters
	return metadata, nil
}
