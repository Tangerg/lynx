package httpreq

import "errors"

var (
	ErrMissingHosts     = errors.New("httpreq: AllowedHosts must not be empty — set explicit allowlist to enable network access")
	ErrMissingRequest   = errors.New("httpreq: request must not be nil")
	ErrEmptyURL         = errors.New("httpreq: url must not be empty")
	ErrInvalidURL       = errors.New("httpreq: url must be an absolute http(s) URL")
	ErrInvalidMethod    = errors.New("httpreq: method must be GET, HEAD, POST, PUT, PATCH, or DELETE")
	ErrInvalidTimeout   = errors.New("httpreq: timeout_ms must be between 1 and 120000 when set")
	ErrInvalidConfig    = errors.New("httpreq: config is invalid")
	ErrHostNotAllowed   = errors.New("httpreq: host is not in AllowedHosts allowlist")
	ErrMethodNotAllowed = errors.New("httpreq: method is not in AllowedMethods allowlist")
)
