package codec

type Codec interface {
	Marshal(data any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

var defaultCodec Codec

func init() {
	SetDefaultCodec(NewJsonCodec())
}

func GetDefaultCodec() Codec {
	return defaultCodec
}

func SetDefaultCodec(c Codec) {
	defaultCodec = c
}

func Marshal(data any) ([]byte, error) {
	return defaultCodec.Marshal(data)
}
func Unmarshal(data []byte, v any) error {
	return defaultCodec.Unmarshal(data, v)
}
