// Package httporigin provides fail-closed HTTP(S) origin comparison for
// credential and redirect boundaries.
package httporigin

import (
	"net"
	"net/url"
	"strings"
)

// Same reports whether two valid HTTP(S) endpoints share the same scheme,
// host, and effective port. Invalid or non-HTTP endpoints never match.
func Same(left, right string) bool {
	leftOrigin, ok := parse(left)
	if !ok {
		return false
	}
	rightOrigin, ok := parse(right)
	return ok && leftOrigin == rightOrigin
}

func parse(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", false
	}
	port := u.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return scheme + "://" + net.JoinHostPort(host, port), true
}
