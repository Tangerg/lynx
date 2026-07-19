package a2a

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
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

func restrictedHTTPClient(base *http.Client, allowed originSet) *http.Client {
	if base == nil {
		base = http.DefaultClient
	}
	client := *base
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client.Transport = &originRoundTripper{base: transport, allowed: allowed}
	previousRedirectPolicy := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := validateAllowedOrigin(req.URL, allowed); err != nil {
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

type originRoundTripper struct {
	base    http.RoundTripper
	allowed originSet
}

func (t *originRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := validateAllowedOrigin(req.URL, t.allowed); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

func validateAllowedOrigin(target *url.URL, allowed originSet) error {
	origin, err := originFromURL(target)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrOriginNotAllowed, err)
	}
	if !allowed.contains(origin) {
		return fmt.Errorf("%w: %s", ErrOriginNotAllowed, origin)
	}
	return nil
}
