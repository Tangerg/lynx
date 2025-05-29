package tool

// Metadata represents metadata about a tool specification and execution.
type Metadata interface {
	// ReturnDirect indicates whether the tool result should be returned directly
	// or passed back to the model for further processing.
	ReturnDirect() bool
}

// metadata is the default implementation of Metadata interface.
type metadata struct {
	returnDirect bool
}

func (m *metadata) ReturnDirect() bool {
	return m.returnDirect
}

// NewMetadata creates a new Metadata instance.
func NewMetadata(returnDirect bool) Metadata {
	return &metadata{returnDirect: returnDirect}
}
