package a2a

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/a2aproject/a2a-go/v2/a2aclient"

	toolcontract "github.com/Tangerg/lynx/tools"
)

// Endpoint describes one remote A2A agent to expose as a chat tool.
type Endpoint struct {
	Name       string
	CardURL    string
	HTTPClient *http.Client
}

// Tools resolves every endpoint and wraps each remote agent as a chat tool.
// The returned close function releases all opened agent clients. It is always
// non-nil and safe to call once startup succeeds.
func Tools(ctx context.Context, endpoints ...Endpoint) ([]toolcontract.Tool, func() error, error) {
	clients := make([]*a2aclient.Client, 0, len(endpoints))
	tools := make([]toolcontract.Tool, 0, len(endpoints))
	seen := make(map[string]struct{}, len(endpoints))
	for _, endpoint := range endpoints {
		client, card, err := dial(ctx, endpoint.CardURL, dialOptions{HTTPClient: endpoint.HTTPClient})
		if err != nil {
			return nil, nil, errors.Join(err, closeClients(clients))
		}
		clients = append(clients, client)

		tool, err := newTool(toolConfig{Client: client, Card: card, Name: endpoint.Name})
		if err != nil {
			return nil, nil, errors.Join(err, closeClients(clients))
		}
		name := tool.Definition().Name
		if _, dup := seen[name]; dup {
			err := fmt.Errorf("a2a.Tools: duplicate tool name %q", name)
			return nil, nil, errors.Join(err, closeClients(clients))
		}
		seen[name] = struct{}{}
		tools = append(tools, tool)
	}
	return tools, func() error { return closeClients(clients) }, nil
}

func closeClients(clients []*a2aclient.Client) error {
	var errs []error
	for _, client := range clients {
		if client == nil {
			continue
		}
		if err := client.Destroy(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
