// Package httporigin provides fail-closed HTTP(S) origin normalization and
// comparison for credential and redirect boundaries.
package httporigin

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Origin is a normalized, comparable HTTP(S) origin: a lowercased scheme and
// host with the effective default port filled in, so two origins are equal iff
// they are the same security origin. The zero value is not a valid origin.
type Origin struct {
	Scheme string // "http" or "https", lowercased
	Host   string // host:port, host lowercased, default port materialized
}

// String renders the origin as "scheme://host:port".
func (o Origin) String() string { return o.Scheme + "://" + o.Host }

// Parse normalizes a raw HTTP(S) URL into an [Origin], failing closed on an
// unparseable URL, a non-HTTP(S) scheme, or a missing host.
func Parse(raw string) (Origin, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Origin{}, fmt.Errorf("parse URL: %w", err)
	}
	return FromURL(u)
}

// FromURL normalizes an already-parsed HTTP(S) URL into an [Origin] (the
// redirect-target path, where the URL is in hand). A nil URL fails closed.
func FromURL(u *url.URL) (Origin, error) {
	if u == nil {
		return Origin{}, errors.New("URL is nil")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return Origin{}, fmt.Errorf("scheme %q is not HTTP or HTTPS", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return Origin{}, errors.New("host is required")
	}
	port := u.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return Origin{Scheme: scheme, Host: net.JoinHostPort(host, port)}, nil
}

// Same reports whether two endpoints share the same origin. Invalid or
// non-HTTP(S) endpoints never match (fail-closed).
func Same(left, right string) bool {
	leftOrigin, err := Parse(left)
	if err != nil {
		return false
	}
	rightOrigin, err := Parse(right)
	return err == nil && leftOrigin == rightOrigin
}
