package httpreq

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/go-resty/resty/v2"
)

// Client executes requests through an immutable network and resource policy.
type Client struct {
	transport        *resty.Client
	allowedHosts     Allowlist
	allowedMethods   map[Method]struct{}
	maxResponseBytes int64
	defaultTimeout   time.Duration
}

func NewClient(config ClientConfig) (*Client, error) {
	policy, err := config.compilePolicy()
	if err != nil {
		return nil, err
	}

	var transport *resty.Client
	if config.HTTPClient != nil {
		// Resty's redirect policy mutates the underlying http.Client. A shallow
		// clone preserves caller ownership while intentionally sharing Transport
		// and Jar, whose concurrency contracts come from net/http.
		httpClient := *config.HTTPClient
		transport = resty.NewWithClient(&httpClient)
	} else {
		transport = resty.New()
	}
	transport.SetRedirectPolicy(resty.RedirectPolicyFunc(policy.allowedHosts.CheckRedirect))
	for name, value := range config.DefaultHeaders {
		transport.SetHeader(name, value)
	}

	return &Client{
		transport:        transport,
		allowedHosts:     policy.allowedHosts,
		allowedMethods:   policy.allowedMethods,
		maxResponseBytes: policy.maxResponseBytes,
		defaultTimeout:   policy.defaultTimeout,
	}, nil
}

// Do applies the frozen host, method, timeout, redirect, and response-size
// policy before returning a model-facing response.
func (client *Client) Do(ctx context.Context, request *Request) (*Response, error) {
	if client == nil {
		return nil, ErrNilClient
	}
	prepared, err := request.prepare()
	if err != nil {
		return nil, err
	}
	method := prepared.Method.Normalize()
	if _, allowed := client.allowedMethods[method]; !allowed {
		return nil, fmt.Errorf("%w: %s", ErrMethodNotAllowed, method)
	}

	parsedURL, err := url.Parse(prepared.URL)
	if err != nil {
		return nil, fmt.Errorf("httpreq: parse validated request URL: %w", err)
	}
	host := parsedURL.Hostname()
	if !client.allowedHosts.Allows(host) {
		return nil, fmt.Errorf("%w: %s", ErrHostNotAllowed, host)
	}

	timeout := client.defaultTimeout
	if prepared.TimeoutMS > 0 {
		timeout = time.Duration(prepared.TimeoutMS) * time.Millisecond
	}
	callContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	restyRequest := client.transport.R().
		SetContext(callContext).
		SetDoNotParseResponse(true)
	for name, value := range prepared.Headers {
		restyRequest.SetHeader(name, value)
	}
	for name, value := range prepared.Query {
		restyRequest.SetQueryParam(name, value)
	}
	if prepared.Body != "" {
		restyRequest.SetBody(prepared.Body)
	}

	startedAt := time.Now()
	response, err := restyRequest.Execute(string(method), prepared.URL)
	duration := time.Since(startedAt)
	if err != nil {
		return nil, fmt.Errorf("httpreq: execute %s request to host %q: %w", method, host, err)
	}
	bodyReader := response.RawBody()
	defer bodyReader.Close()
	body, truncated, err := readCapped(bodyReader, client.maxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("httpreq: read response body from host %q: %w", host, err)
	}

	return &Response{
		Status:    response.StatusCode(),
		Headers:   response.Header().Clone(),
		Body:      string(body),
		Truncated: truncated,
		Duration:  duration.String(),
	}, nil
}
