package dispatch

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

const requestMetaField = "_meta"

func bindRequestMeta(ctx context.Context, req *transport.Request) (context.Context, *transport.Error) {
	if req == nil || len(req.Params) == 0 {
		return ctx, nil
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(req.Params, &obj); err != nil {
		return ctx, nil
	}

	raw, ok := obj[requestMetaField]
	if !ok {
		return ctx, nil
	}

	var metaObject map[string]json.RawMessage
	if err := json.Unmarshal(raw, &metaObject); err != nil {
		return ctx, invalidParams(requestMetaField + ": " + err.Error())
	}
	if metaObject == nil {
		return ctx, invalidParams(requestMetaField + ": must be an object")
	}

	var meta protocol.RequestMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return ctx, invalidParams(requestMetaField + ": " + err.Error())
	}
	if meta.ProtocolVersion != "" && meta.ProtocolVersion != protocol.ProtocolVersion {
		return ctx, problemError(
			protocol.CodeInvalidProtocolVersion,
			protocol.ErrInvalidProtocolVersion.Error(),
			fmt.Sprintf("protocolVersion %q is unsupported; expected %q", meta.ProtocolVersion, protocol.ProtocolVersion),
		)
	}

	delete(obj, requestMetaField)
	if len(obj) == 0 {
		req.Params = nil
	} else {
		stripped, err := json.Marshal(obj)
		if err != nil {
			return ctx, invalidParams(requestMetaField + ": " + err.Error())
		}
		req.Params = stripped
	}

	return protocol.WithRequestMeta(ctx, meta), nil
}
