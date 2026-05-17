package bedrockkb

import "errors"

// ErrUnsupported signals that the call site (Create / Delete) is
// not implemented by this store. Bedrock Knowledge Bases ingest
// documents via configured data sources (S3, Confluence, …), not the
// runtime API — callers should pair lynx with the appropriate data
// source mechanism.
var ErrUnsupported = errors.New("bedrockkb: documents are managed via data-source sync, not directly")
