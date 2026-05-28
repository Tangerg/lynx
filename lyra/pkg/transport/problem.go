package transport

// ProblemData mirrors RFC 7807 ProblemDetails, used as the Data
// payload on [Error] (API.md §7.2). Most fields are optional — only
// `detail` is set for ordinary errors; `errors[]` shows up on
// validation failures; `retryAfterMs` shows up on rate-limit codes.
type ProblemData struct {
	Type         string       `json:"type,omitempty"`
	Detail       string       `json:"detail,omitempty"`
	RetryAfterMs int          `json:"retryAfterMs,omitempty"`
	Errors       []FieldError `json:"errors,omitempty"`
}

// FieldError is one entry in ProblemData.Errors — used to point at a
// specific field in invalid params.
type FieldError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}
