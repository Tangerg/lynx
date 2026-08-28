package httpreq

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultTimeout          = 30 * time.Second
	DefaultMaxResponseBytes = int64(256 * 1024)
	MaxRequestTimeout       = 2 * time.Minute
)

// ClientConfig defines the network authority and resource bounds frozen into
// a Client. AllowedHosts is mandatory because the zero policy denies network
// access rather than silently opening it.
type ClientConfig struct {
	// AllowedHosts accepts exact hosts and one leading wildcard, such as
	// "api.example.com" or "*.example.com". A wildcard does not match its root.
	AllowedHosts []string

	// AllowedMethods defaults to GET and HEAD. Comparison is case-insensitive.
	AllowedMethods []Method

	// DefaultHeaders are added unless [Request.Headers] overrides them.
	DefaultHeaders map[string]string

	// MaxResponseBytes selects [DefaultMaxResponseBytes] at zero.
	MaxResponseBytes int64

	// DefaultTimeout selects [DefaultTimeout] at zero.
	DefaultTimeout time.Duration

	// HTTPClient supplies caller-owned transport, cookie jar, proxy, and TLS
	// settings. NewClient clones the value before installing redirect policy.
	HTTPClient *http.Client
}

type clientPolicy struct {
	allowedHosts     Allowlist
	allowedMethods   map[Method]struct{}
	maxResponseBytes int64
	defaultTimeout   time.Duration
}

func (config ClientConfig) Validate() error {
	_, err := config.compilePolicy()
	return err
}

func (config ClientConfig) compilePolicy() (clientPolicy, error) {
	if len(config.AllowedHosts) == 0 {
		return clientPolicy{}, fmt.Errorf("%w: %w", ErrInvalidClientConfig, ErrMissingAllowedHosts)
	}
	allowedHosts, err := NewAllowlist(config.AllowedHosts)
	if err != nil {
		return clientPolicy{}, fmt.Errorf("%w: allowed hosts: %w", ErrInvalidClientConfig, err)
	}

	methods := config.AllowedMethods
	if len(methods) == 0 {
		methods = []Method{MethodGET, MethodHEAD}
	}
	allowedMethods := make(map[Method]struct{}, len(methods))
	for index, method := range methods {
		if strings.TrimSpace(string(method)) == "" {
			return clientPolicy{}, fmt.Errorf("%w: allowed method %d is blank", ErrInvalidClientConfig, index)
		}
		if err := method.Validate(); err != nil {
			return clientPolicy{}, fmt.Errorf(
				"%w: allowed method %d %q: %w",
				ErrInvalidClientConfig,
				index,
				method,
				err,
			)
		}
		allowedMethods[method.Normalize()] = struct{}{}
	}
	if config.DefaultTimeout < 0 {
		return clientPolicy{}, fmt.Errorf("%w: default timeout must not be negative", ErrInvalidClientConfig)
	}
	if config.MaxResponseBytes < 0 {
		return clientPolicy{}, fmt.Errorf("%w: maximum response bytes must not be negative", ErrInvalidClientConfig)
	}

	maxResponseBytes := config.MaxResponseBytes
	if maxResponseBytes == 0 {
		maxResponseBytes = DefaultMaxResponseBytes
	}
	defaultTimeout := config.DefaultTimeout
	if defaultTimeout == 0 {
		defaultTimeout = DefaultTimeout
	}
	return clientPolicy{
		allowedHosts:     allowedHosts,
		allowedMethods:   allowedMethods,
		maxResponseBytes: maxResponseBytes,
		defaultTimeout:   defaultTimeout,
	}, nil
}
