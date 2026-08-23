// Package mcpserver owns durable MCP configuration, secret mutation semantics,
// authorization attempts, and the two identities of a remote tool. Transport
// sessions and wire presentation belong to adapters outside this package.
package mcpserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	maxNameBytes        = 128
	maxDescriptionBytes = 4 << 10
	maxEndpointBytes    = 8 << 10
	maxCommandBytes     = 4 << 10
	maxDirectoryBytes   = 8 << 10
	maxArgumentBytes    = 8 << 10
	maxArguments        = 256
	maxSecretBytes      = 64 << 10
	maxSecretEntries    = 256
	maxToolPolicies     = 2_048
)

var (
	ErrInvalid          = errors.New("mcpserver: invalid configuration")
	ErrNotFound         = errors.New("mcpserver: not found")
	ErrExists           = errors.New("mcpserver: already exists")
	ErrRevisionConflict = errors.New("mcpserver: revision conflict")
	ErrUnavailable      = errors.New("mcpserver: unavailable")
)

// Transport is the closed durable connection vocabulary. It intentionally
// does not depend on the public wire package.
type Transport string

const (
	TransportStdio          Transport = "stdio"
	TransportStreamableHTTP Transport = "streamableHttp"
)

// SecretChange gives a secret-bearing field exact keep/set/clear semantics.
// Its zero value is Keep, which is how an omitted wire member is represented.
type SecretChange[T any] struct {
	Set   bool
	Clear bool
	Value T
}

// ConnectionPatch atomically replaces all safe connection fields. Secrets are
// independently changed, except that switching transports clears credentials
// from the previous transport by construction.
type ConnectionPatch struct {
	Transport     Transport
	URL           string
	Command       string
	Args          []string
	Dir           string
	Authorization SecretChange[string]
	Headers       SecretChange[map[string]string]
	Environment   SecretChange[map[string]string]
}

// Patch is one intent against a Configuration generation. Nil fields preserve
// current state; present empty values explicitly clear safe optional fields.
type Patch struct {
	Enabled          *bool
	Description      *string
	Connection       *ConnectionPatch
	TimeoutSeconds   *int
	DisabledTools    *[]string
	AutoApproveTools *[]string
}

// State is the storage adapter representation of one aggregate. It is not a
// wire DTO: in particular Secrets must never be projected to a client.
type State struct {
	Name             string      `json:"name"`
	Enabled          bool        `json:"enabled"`
	Description      string      `json:"description,omitempty"`
	Transport        Transport   `json:"transport"`
	URL              string      `json:"url,omitempty"`
	Command          string      `json:"command,omitempty"`
	Args             []string    `json:"args,omitempty"`
	Dir              string      `json:"dir,omitempty"`
	TimeoutSeconds   int         `json:"timeoutSeconds,omitempty"`
	DisabledTools    []string    `json:"disabledTools,omitempty"`
	AutoApproveTools []string    `json:"autoApproveTools,omitempty"`
	Secrets          SecretState `json:"-"`
	Revision         uint64      `json:"revision"`
	UpdatedAt        time.Time   `json:"updatedAt"`
}

// SecretState is stored in SQLite's protected secret column, separate from the
// safe JSON body. Callers receive clones from Configuration.State.
type SecretState struct {
	Authorization string            `json:"authorization,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	Environment   map[string]string `json:"environment,omitempty"`
	OAuthOrigin   string            `json:"oauthOrigin,omitempty"`
	OAuthSession  []byte            `json:"oauthSession,omitempty"`
}

// Configuration is the durable aggregate for one named MCP integration.
type Configuration struct {
	state State
}

func New(name string, patch Patch, now time.Time) (Configuration, error) {
	value := Configuration{state: State{
		Name: strings.TrimSpace(name), Revision: 1, UpdatedAt: now.UTC(),
	}}
	if patch.Connection == nil {
		return Configuration{}, fmt.Errorf("%w: connection is required", ErrInvalid)
	}
	if err := value.apply(patch); err != nil {
		return Configuration{}, err
	}
	value.state.Revision = 1
	value.state.UpdatedAt = now.UTC()
	if err := value.validate(); err != nil {
		return Configuration{}, err
	}
	return value, nil
}

func Rehydrate(state State) (Configuration, error) {
	state.Args = slices.Clone(state.Args)
	state.DisabledTools = slices.Clone(state.DisabledTools)
	state.AutoApproveTools = slices.Clone(state.AutoApproveTools)
	state.Secrets = cloneSecrets(state.Secrets)
	value := Configuration{state: state}
	if err := value.validate(); err != nil {
		return Configuration{}, err
	}
	return value, nil
}

// Apply replays one exact patch against the current generation. No-op intent
// consumes neither a revision nor an updatedAt value.
func (value *Configuration) Apply(patch Patch, now time.Time) (bool, error) {
	next, err := Rehydrate(value.State())
	if err != nil {
		return false, err
	}
	if err := next.apply(patch); err != nil {
		return false, err
	}
	if err := next.validate(); err != nil {
		return false, err
	}
	if equalState(value.state, next.state) {
		return false, nil
	}
	next.state.Revision = value.state.Revision + 1
	next.state.UpdatedAt = now.UTC()
	value.state = next.state
	return true, nil
}

func (value *Configuration) apply(patch Patch) error {
	if patch.Enabled != nil {
		value.state.Enabled = *patch.Enabled
	}
	if patch.Description != nil {
		value.state.Description = strings.TrimSpace(*patch.Description)
	}
	if patch.TimeoutSeconds != nil {
		value.state.TimeoutSeconds = *patch.TimeoutSeconds
	}
	if patch.DisabledTools != nil {
		value.state.DisabledTools = normalizeNames(*patch.DisabledTools)
	}
	if patch.AutoApproveTools != nil {
		value.state.AutoApproveTools = normalizeNames(*patch.AutoApproveTools)
	}
	if patch.Connection == nil {
		return nil
	}
	connection := patch.Connection
	previousTransport := value.state.Transport
	previousURL := value.state.URL
	if previousTransport != "" && previousTransport != connection.Transport {
		value.state.Secrets = SecretState{}
	}
	value.state.Transport = connection.Transport
	value.state.URL = strings.TrimSpace(connection.URL)
	value.state.Command = strings.TrimSpace(connection.Command)
	value.state.Args = slices.Clone(connection.Args)
	value.state.Dir = strings.TrimSpace(connection.Dir)
	if err := applySecret(&value.state.Secrets.Authorization, connection.Authorization); err != nil {
		return fmt.Errorf("%w: authorization: %v", ErrInvalid, err)
	}
	if err := applySecretMap(&value.state.Secrets.Headers, connection.Headers); err != nil {
		return fmt.Errorf("%w: headers: %v", ErrInvalid, err)
	}
	if err := applySecretMap(&value.state.Secrets.Environment, connection.Environment); err != nil {
		return fmt.Errorf("%w: environment: %v", ErrInvalid, err)
	}
	clearOAuth := connection.Authorization.Set || connection.Authorization.Clear ||
		hasAuthorizationHeader(connection.Headers)
	if previousTransport != "" && (previousTransport != value.state.Transport ||
		!sameEndpointOrigin(previousURL, value.state.URL)) {
		clearOAuth = true
	}
	if clearOAuth {
		value.state.Secrets.OAuthOrigin = ""
		value.state.Secrets.OAuthSession = nil
	}
	return nil
}

// PutOAuth accepts a token only for the aggregate generation that initiated the
// authorization flow. The storage CAS supplies the cross-process half of that
// invariant; origin binding supplies the endpoint half.
func (value *Configuration) PutOAuth(origin string, payload []byte, now time.Time) (bool, error) {
	currentOrigin, err := value.HTTPOrigin()
	if err != nil || origin == "" || origin != currentOrigin {
		return false, fmt.Errorf("%w: OAuth session origin is stale", ErrInvalid)
	}
	if len(payload) == 0 || len(payload) > maxSecretBytes {
		return false, fmt.Errorf("%w: OAuth session payload is invalid", ErrInvalid)
	}
	next := value.State()
	next.Secrets.Authorization = ""
	for name := range next.Secrets.Headers {
		if strings.EqualFold(name, "Authorization") {
			delete(next.Secrets.Headers, name)
		}
	}
	next.Secrets.OAuthOrigin = origin
	next.Secrets.OAuthSession = bytes.Clone(payload)
	if equalState(value.state, next) {
		return false, nil
	}
	next.Revision = value.state.Revision + 1
	next.UpdatedAt = now.UTC()
	value.state = next
	return true, nil
}

func (value *Configuration) ClearOAuth(now time.Time) bool {
	if value.state.Secrets.OAuthOrigin == "" && len(value.state.Secrets.OAuthSession) == 0 {
		return false
	}
	value.state.Secrets.OAuthOrigin = ""
	value.state.Secrets.OAuthSession = nil
	value.state.Revision++
	value.state.UpdatedAt = now.UTC()
	return true
}

func (value Configuration) validate() error {
	state := value.state
	switch {
	case state.Name == "" || strings.TrimSpace(state.Name) != state.Name:
		return fmt.Errorf("%w: name is required", ErrInvalid)
	case len(state.Name) > maxNameBytes || containsControl(state.Name):
		return fmt.Errorf("%w: name is unsafe", ErrInvalid)
	case len(state.Description) > maxDescriptionBytes || !utf8.ValidString(state.Description):
		return fmt.Errorf("%w: description is too large", ErrInvalid)
	case state.TimeoutSeconds < 0 || state.TimeoutSeconds > 3_600:
		return fmt.Errorf("%w: timeout must be between 0 and 3600 seconds", ErrInvalid)
	case state.Revision == 0:
		return fmt.Errorf("%w: revision must be positive", ErrInvalid)
	case state.UpdatedAt.IsZero():
		return fmt.Errorf("%w: updated time is required", ErrInvalid)
	}
	if err := validateToolPolicies(state.DisabledTools, state.AutoApproveTools); err != nil {
		return err
	}
	if err := validateSecret(state.Secrets.Authorization); err != nil {
		return fmt.Errorf("%w: authorization: %v", ErrInvalid, err)
	}
	if err := validateMap(state.Secrets.Headers, true); err != nil {
		return fmt.Errorf("%w: headers: %v", ErrInvalid, err)
	}
	if err := validateMap(state.Secrets.Environment, false); err != nil {
		return fmt.Errorf("%w: environment: %v", ErrInvalid, err)
	}
	if len(state.Secrets.OAuthSession) > maxSecretBytes {
		return fmt.Errorf("%w: OAuth session is too large", ErrInvalid)
	}
	switch state.Transport {
	case TransportStreamableHTTP:
		parsed, err := url.ParseRequestURI(state.URL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
			parsed.User != nil || parsed.Fragment != "" || len(state.URL) > maxEndpointBytes {
			return fmt.Errorf("%w: streamable HTTP endpoint is invalid", ErrInvalid)
		}
		if state.Command != "" || len(state.Args) != 0 || state.Dir != "" || len(state.Secrets.Environment) != 0 {
			return fmt.Errorf("%w: streamable HTTP contains stdio fields", ErrInvalid)
		}
		if len(state.Secrets.OAuthSession) > 0 &&
			(state.Secrets.Authorization != "" || containsAuthorizationHeader(state.Secrets.Headers)) {
			return fmt.Errorf("%w: OAuth and explicit authorization cannot coexist", ErrInvalid)
		}
		origin, originErr := value.HTTPOrigin()
		if originErr != nil || (state.Secrets.OAuthOrigin != "" && state.Secrets.OAuthOrigin != origin) ||
			((state.Secrets.OAuthOrigin == "") != (len(state.Secrets.OAuthSession) == 0)) {
			return fmt.Errorf("%w: OAuth session is not bound to the endpoint", ErrInvalid)
		}
	case TransportStdio:
		if state.Command == "" || len(state.Command) > maxCommandBytes || containsNUL(state.Command) {
			return fmt.Errorf("%w: stdio command is invalid", ErrInvalid)
		}
		if state.URL != "" || state.Secrets.Authorization != "" || len(state.Secrets.Headers) != 0 ||
			state.Secrets.OAuthOrigin != "" || len(state.Secrets.OAuthSession) != 0 {
			return fmt.Errorf("%w: stdio contains HTTP fields", ErrInvalid)
		}
		if state.Dir != "" && (!filepath.IsAbs(state.Dir) || len(state.Dir) > maxDirectoryBytes || containsNUL(state.Dir)) {
			return fmt.Errorf("%w: stdio working directory must be absolute", ErrInvalid)
		}
		if len(state.Args) > maxArguments {
			return fmt.Errorf("%w: too many stdio arguments", ErrInvalid)
		}
		for _, argument := range state.Args {
			if len(argument) > maxArgumentBytes || containsNUL(argument) {
				return fmt.Errorf("%w: stdio argument is invalid", ErrInvalid)
			}
		}
	default:
		return fmt.Errorf("%w: unknown transport", ErrInvalid)
	}
	return nil
}

func (value Configuration) State() State {
	state := value.state
	state.Args = slices.Clone(state.Args)
	state.DisabledTools = slices.Clone(state.DisabledTools)
	state.AutoApproveTools = slices.Clone(state.AutoApproveTools)
	state.Secrets = cloneSecrets(state.Secrets)
	return state
}

func (value Configuration) Clone() Configuration {
	return Configuration{state: value.State()}
}

func (value Configuration) Name() string            { return value.state.Name }
func (value Configuration) Enabled() bool           { return value.state.Enabled }
func (value Configuration) Description() string     { return value.state.Description }
func (value Configuration) Transport() Transport    { return value.state.Transport }
func (value Configuration) URL() string             { return value.state.URL }
func (value Configuration) Command() string         { return value.state.Command }
func (value Configuration) Args() []string          { return slices.Clone(value.state.Args) }
func (value Configuration) Dir() string             { return value.state.Dir }
func (value Configuration) TimeoutSeconds() int     { return value.state.TimeoutSeconds }
func (value Configuration) DisabledTools() []string { return slices.Clone(value.state.DisabledTools) }
func (value Configuration) AutoApproveTools() []string {
	return slices.Clone(value.state.AutoApproveTools)
}
func (value Configuration) Revision() uint64     { return value.state.Revision }
func (value Configuration) UpdatedAt() time.Time { return value.state.UpdatedAt }
func (value Configuration) Secrets() SecretState { return cloneSecrets(value.state.Secrets) }

func (value Configuration) HTTPOrigin() (string, error) {
	if value.state.Transport != TransportStreamableHTTP {
		return "", fmt.Errorf("%w: transport has no HTTP origin", ErrInvalid)
	}
	return endpointOrigin(value.state.URL)
}

// ToolName deterministically collapses the lossless (server, remote) identity
// into the model's 64-character namespace. Execution keeps the original pair;
// only the model-visible definition uses this value.
func ToolName(server, remote string) (string, error) {
	if strings.TrimSpace(server) != server || strings.TrimSpace(remote) != remote || server == "" || remote == "" ||
		len(server) > maxNameBytes || len(remote) > 512 || containsControl(server) || containsControl(remote) {
		return "", fmt.Errorf("%w: safe, bounded tool identity is required", ErrInvalid)
	}
	digest := sha256.Sum256([]byte(server + "\x00" + remote))
	stem := strings.Trim(invalidToolName.ReplaceAllString(server+"_"+remote, "_"), "_")
	const suffixLength = 13
	maximumStem := 64 - len("mcp_") - suffixLength
	if len(stem) > maximumStem {
		stem = stem[:maximumStem]
	}
	if stem == "" {
		stem = "tool"
	}
	return "mcp_" + stem + "_" + hex.EncodeToString(digest[:6]), nil
}

var invalidToolName = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func applySecret(target *string, change SecretChange[string]) error {
	if change.Set && change.Clear {
		return errors.New("set and clear are mutually exclusive")
	}
	switch {
	case change.Clear:
		*target = ""
	case change.Set:
		if err := validateSecret(change.Value); err != nil {
			return err
		}
		*target = change.Value
	}
	return nil
}

func applySecretMap(target *map[string]string, change SecretChange[map[string]string]) error {
	if change.Set && change.Clear {
		return errors.New("set and clear are mutually exclusive")
	}
	switch {
	case change.Clear:
		*target = nil
	case change.Set:
		if len(change.Value) == 0 {
			return errors.New("set requires at least one entry")
		}
		*target = maps.Clone(change.Value)
	}
	return nil
}

func validateSecret(value string) error {
	if len(value) > maxSecretBytes || !utf8.ValidString(value) || strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("value is unsafe or too large")
	}
	return nil
}

func validateMap(values map[string]string, headers bool) error {
	if len(values) > maxSecretEntries {
		return errors.New("too many entries")
	}
	for name, value := range values {
		if name == "" || len(name) > 256 || len(value) > maxSecretBytes || containsNUL(name) || containsNUL(value) {
			return errors.New("entry is unsafe or too large")
		}
		if headers {
			if !validHeaderName(name) || strings.ContainsAny(value, "\r\n") {
				return errors.New("HTTP header is invalid")
			}
		} else if strings.Contains(name, "=") {
			return errors.New("environment name is invalid")
		}
	}
	return nil
}

func validateToolPolicies(disabled, autoApprove []string) error {
	if len(disabled) > maxToolPolicies || len(autoApprove) > maxToolPolicies {
		return fmt.Errorf("%w: too many tool policies", ErrInvalid)
	}
	seen := make(map[string]bool, len(disabled)+len(autoApprove))
	for _, collection := range [][]string{disabled, autoApprove} {
		for _, name := range collection {
			if name == "" || strings.TrimSpace(name) != name || len(name) > 512 || containsControl(name) || seen[name] {
				return fmt.Errorf("%w: tool policies must be unique, safe, and disjoint", ErrInvalid)
			}
			seen[name] = true
		}
	}
	return nil
}

func normalizeNames(values []string) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = strings.TrimSpace(value)
	}
	slices.Sort(result)
	return result
}

func equalState(left, right State) bool {
	return left.Name == right.Name && left.Enabled == right.Enabled &&
		left.Description == right.Description && left.Transport == right.Transport &&
		left.URL == right.URL && left.Command == right.Command && slices.Equal(left.Args, right.Args) &&
		left.Dir == right.Dir && left.TimeoutSeconds == right.TimeoutSeconds &&
		slices.Equal(left.DisabledTools, right.DisabledTools) &&
		slices.Equal(left.AutoApproveTools, right.AutoApproveTools) &&
		left.Secrets.Authorization == right.Secrets.Authorization &&
		maps.Equal(left.Secrets.Headers, right.Secrets.Headers) &&
		maps.Equal(left.Secrets.Environment, right.Secrets.Environment) &&
		left.Secrets.OAuthOrigin == right.Secrets.OAuthOrigin &&
		bytes.Equal(left.Secrets.OAuthSession, right.Secrets.OAuthSession)
}

func cloneSecrets(value SecretState) SecretState {
	value.Headers = maps.Clone(value.Headers)
	value.Environment = maps.Clone(value.Environment)
	value.OAuthSession = bytes.Clone(value.OAuthSession)
	return value
}

func hasAuthorizationHeader(change SecretChange[map[string]string]) bool {
	if !change.Set {
		return false
	}
	for name := range change.Value {
		if strings.EqualFold(name, "Authorization") {
			return true
		}
	}
	return false
}

func containsAuthorizationHeader(values map[string]string) bool {
	for name := range values {
		if strings.EqualFold(name, "Authorization") {
			return true
		}
	}
	return false
}

func endpointOrigin(endpoint string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("%w: invalid HTTP origin", ErrInvalid)
	}
	return (&url.URL{Scheme: strings.ToLower(parsed.Scheme), Host: strings.ToLower(parsed.Host)}).String(), nil
}

func sameEndpointOrigin(left, right string) bool {
	leftOrigin, leftErr := endpointOrigin(left)
	rightOrigin, rightErr := endpointOrigin(right)
	return leftErr == nil && rightErr == nil && leftOrigin == rightOrigin
}

func containsControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0 || !utf8.ValidString(value)
}

func containsNUL(value string) bool { return strings.ContainsRune(value, 0) }

func validHeaderName(value string) bool {
	for _, character := range value {
		if character > unicode.MaxASCII || !(unicode.IsLetter(character) || unicode.IsDigit(character) ||
			strings.ContainsRune("!#$%&'*+-.^_`|~", character)) {
			return false
		}
	}
	return true
}
