package a2a

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
)

const maxHTTPRedirects = 10

type httpOrigin struct {
	scheme string
	host   string
}

func (o httpOrigin) String() string {
	return o.scheme + "://" + o.host
}

type originSet map[httpOrigin]struct{}

func (s originSet) contains(origin httpOrigin) bool {
	_, ok := s[origin]
	return ok
}

func (s originSet) validate(target *url.URL) error {
	origin, err := originFromURL(target)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrOriginNotAllowed, err)
	}
	if !s.contains(origin) {
		return fmt.Errorf("%w: %s", ErrOriginNotAllowed, origin)
	}
	return nil
}

func (s originSet) restrict(base *http.Client) *http.Client {
	client := *base
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client.Transport = &originRoundTripper{base: transport, allowed: s}
	previousRedirectPolicy := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := s.validate(req.URL); err != nil {
			return err
		}
		if previousRedirectPolicy != nil {
			return previousRedirectPolicy(req, via)
		}
		if len(via) >= maxHTTPRedirects {
			return fmt.Errorf("a2a: stopped after %d redirects", maxHTTPRedirects)
		}
		return nil
	}
	return &client
}

type endpointOriginPolicy struct {
	cardOrigins originSet
	rpcOrigins  originSet
}

func newEndpointOriginPolicy(cardURL string, allowedRPCOrigins []string) (endpointOriginPolicy, error) {
	cardOrigin, err := originFromURLString(cardURL)
	if err != nil {
		return endpointOriginPolicy{}, fmt.Errorf("%w %q: %v", ErrInvalidCardURL, cardURL, err)
	}
	policy := endpointOriginPolicy{
		cardOrigins: originSet{cardOrigin: {}},
		rpcOrigins:  originSet{cardOrigin: {}},
	}
	for _, rawOrigin := range allowedRPCOrigins {
		origin, err := parseConfiguredOrigin(rawOrigin)
		if err != nil {
			return endpointOriginPolicy{}, fmt.Errorf("%w %q: %v", ErrInvalidRPCOrigin, rawOrigin, err)
		}
		policy.rpcOrigins[origin] = struct{}{}
	}
	return policy, nil
}

func (p endpointOriginPolicy) validateCard(card *sdka2a.AgentCard) error {
	for i, iface := range card.SupportedInterfaces {
		if iface == nil {
			return fmt.Errorf("%w: supported interface %d is nil", ErrInvalidCard, i)
		}
		switch iface.ProtocolBinding {
		case sdka2a.TransportProtocolJSONRPC, sdka2a.TransportProtocolHTTPJSON:
		default:
			continue
		}
		origin, err := originFromURLString(iface.URL)
		if err != nil {
			return fmt.Errorf("%w: supported interface %d URL %q: %v", ErrInvalidCard, i, iface.URL, err)
		}
		if !p.rpcOrigins.contains(origin) {
			return fmt.Errorf("%w: supported interface %d uses %s", ErrOriginNotAllowed, i, origin)
		}
	}
	return nil
}

func originFromURLString(rawURL string) (httpOrigin, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return httpOrigin{}, fmt.Errorf("parse URL: %w", err)
	}
	return originFromURL(u)
}

func parseConfiguredOrigin(rawOrigin string) (httpOrigin, error) {
	u, err := url.Parse(rawOrigin)
	if err != nil {
		return httpOrigin{}, fmt.Errorf("parse URL: %w", err)
	}
	if u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return httpOrigin{}, errors.New("must contain only scheme and host")
	}
	return originFromURL(u)
}

func originFromURL(u *url.URL) (httpOrigin, error) {
	if u == nil {
		return httpOrigin{}, errors.New("URL is nil")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return httpOrigin{}, fmt.Errorf("scheme %q is not HTTP or HTTPS", u.Scheme)
	}
	hostname := strings.ToLower(u.Hostname())
	if hostname == "" {
		return httpOrigin{}, errors.New("host is required")
	}
	port := u.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return httpOrigin{scheme: scheme, host: net.JoinHostPort(hostname, port)}, nil
}

type originRoundTripper struct {
	base    http.RoundTripper
	allowed originSet
}

func (t *originRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.allowed.validate(req.URL); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}
