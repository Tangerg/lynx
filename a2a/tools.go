package a2a

import (
	"context"
	"errors"
	"fmt"

	"github.com/a2aproject/a2a-go/v2/a2aclient"

	toolcontract "github.com/Tangerg/scope/core/tool"
)

// OpenTools resolves every endpoint, opens its client, and wraps each remote
// agent as a chat tool.
// The returned close function releases all opened agent clients. It is always
// non-nil and safe to call once startup succeeds.
func OpenTools(ctx context.Context, endpoints ...Endpoint) ([]toolcontract.Tool, func() error, error) {
	var clients []*a2aclient.Client
	var tools []toolcontract.Tool
	seen := make(map[string]struct{}, len(endpoints))
	for _, endpoint := range endpoints {
		client, card, err := dial(ctx, endpoint)
		if err != nil {
			return nil, nil, errors.Join(err, closeClients(clients))
		}
		clients = append(clients, client)

		remote, err := newRemoteTool(remoteToolConfig{client: client, card: card, name: endpoint.Name})
		if err != nil {
			return nil, nil, errors.Join(err, closeClients(clients))
		}
		name := remote.Definition().Name
		if _, dup := seen[name]; dup {
			err := fmt.Errorf("a2a: duplicate remote-agent tool name %q", name)
			return nil, nil, errors.Join(err, closeClients(clients))
		}
		seen[name] = struct{}{}
		tools = append(tools, remote)
	}
	return tools, func() error { return closeClients(clients) }, nil
}

func closeClients(clients []*a2aclient.Client) error {
	var errs []error
	for _, client := range clients {
		if err := client.Destroy(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
