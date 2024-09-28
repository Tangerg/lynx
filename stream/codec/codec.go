package codec

// Codec is an interface that defines methods for marshaling and unmarshaling data.
type Codec interface {
	// Marshal converts a data structure into a byte slice.
	Marshal(data any) ([]byte, error)
	// Unmarshal parses a byte slice into a data structure.
	Unmarshal(data []byte, v any) error
}

// defaultCodec is a package-level variable that holds the default Codec implementation.
var defaultCodec Codec

// init initializes the package by setting the default Codec to a new JSON codec.
func init() {
	SetDefaultCodec(NewJsonCodec())
}

// GetDefaultCodec returns the current default Codec implementation.
func GetDefaultCodec() Codec {
	return defaultCodec
}

// SetDefaultCodec sets the default Codec implementation to the provided Codec.
func SetDefaultCodec(c Codec) {
	defaultCodec = c
}

// Marshal uses the default Codec to marshal the given data into a byte slice.
func Marshal(data any) ([]byte, error) {
	return defaultCodec.Marshal(data)
}

// Unmarshal uses the default Codec to unmarshal the given byte slice into the provided data structure.
func Unmarshal(data []byte, v any) error {
	return defaultCodec.Unmarshal(data, v)
}
