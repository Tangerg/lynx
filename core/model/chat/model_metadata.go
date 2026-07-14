package chat

// ModelMetadata is the legacy Chat client identity hint. Model catalog,
// pricing, and capability data live in models/catalog and are intentionally
// not part of the invocation SPI.
type ModelMetadata struct {
	Provider string `json:"provider"`
}
