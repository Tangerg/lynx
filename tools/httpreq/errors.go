package httpreq

import "errors"

var (
	ErrMissingConfig   = errors.New("httpreq: config must not be nil")
	ErrMissingHosts    = errors.New("httpreq: AllowedHosts must not be empty — set explicit allowlist to enable network access")
	ErrMissingRequest  = errors.New("httpreq: request must not be nil")
	ErrEmptyURL        = errors.New("httpreq: url must not be empty")
	ErrInvalidURL      = errors.New("httpreq: url must be an absolute http(s) URL")
	ErrHostNotAllowed  = errors.New("httpreq: host is not in AllowedHosts allowlist")
	ErrMethodNotAllowed = errors.New("httpreq: method is not in AllowedMethods allowlist")
)
