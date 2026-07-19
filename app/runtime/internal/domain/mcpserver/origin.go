package mcpserver

import (
	"net"
	"net/url"
	"strings"
)

// SameHTTPOrigin reports whether two valid HTTP(S) endpoints share the same
// scheme, host, and effective port. Invalid or non-HTTP endpoints never match;
// credential preservation must fail closed.
func SameHTTPOrigin(left, right string) bool {
	leftOrigin, ok := httpOrigin(left)
	if !ok {
		return false
	}
	rightOrigin, ok := httpOrigin(right)
	return ok && leftOrigin == rightOrigin
}

func httpOrigin(raw string) (string, bool) {
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
