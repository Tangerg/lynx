package dispatch

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

const requestMetaField = "_meta"

func bindRequestMeta(ctx context.Context, request *transport.Request) (context.Context, *transport.Error) {
	if request == nil || len(request.Params) == 0 {
		return ctx, nil
	}

	var parameters map[string]json.RawMessage
	if err := json.Unmarshal(request.Params, &parameters); err != nil {
		return ctx, nil
	}

	encodedMetadata, ok := parameters[requestMetaField]
	if !ok {
		return ctx, nil
	}

	var metadata protocol.RequestMeta
	if err := decodeParams(encodedMetadata, &metadata); err != nil {
		return ctx, invalidParams(requestMetaField + ": " + err.Error())
	}
	if err := protocol.ValidateWireTree(metadata); err != nil {
		return ctx, invalidRequestShape(err)
	}
	if metadata.ProtocolVersion != "" && !protocol.SupportsProtocolVersion(metadata.ProtocolVersion) {
		return ctx, problemError(
			protocol.ErrInvalidProtocolVersion,
			fmt.Sprintf("protocolVersion %q is unsupported; supported range is %q through %q", metadata.ProtocolVersion, protocol.MinProtocolVersion, protocol.ProtocolVersion),
		)
	}

	delete(parameters, requestMetaField)
	if len(parameters) == 0 {
		request.Params = nil
	} else {
		encodedParameters, err := json.Marshal(parameters)
		if err != nil {
			return ctx, invalidParams(requestMetaField + ": " + err.Error())
		}
		request.Params = encodedParameters
	}

	return protocol.WithRequestMeta(ctx, metadata), nil
}
