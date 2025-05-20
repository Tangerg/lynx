package content

// Content represents data with text and associated metadata.
// It serves as the base interface for content types in the system.
type Content interface {
	// Text returns the textual representation of the content
	Text() string

	// Metadata returns additional information about the content as a key-value map
	Metadata() map[string]any
}

// MediaContent extends the Content interface to include media attachments.
// It can represent rich content that contains both text and media elements.
type MediaContent interface {
	Content

	// Media returns the slice of media objects associated with this content
	Media() []*Media
}
