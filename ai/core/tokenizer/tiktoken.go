package tokenizer

import (
	"github.com/pkoukk/tiktoken-go"
)

var _ Tokenizer = (*Tiktoken)(nil)

type Tiktoken struct {
	encodingType string
	encoding     *tiktoken.Tiktoken
}

func (t *Tiktoken) EncodingType() string {
	return t.encodingType
}

func (t *Tiktoken) Estimate(text string) int {
	return len(t.EncodeTokens(text))
}

func (t *Tiktoken) EstimateTokens(text string) (int, []int) {
	token := t.EncodeTokens(text)
	return len(token), token
}

func (t *Tiktoken) EncodeTokens(text string) []int {
	return t.encoding.Encode(text, nil, nil)
}

func (t *Tiktoken) DecodeTokens(tokens []int) string {
	return t.encoding.Decode(tokens)
}

func NewTiktoken(encodingType string) (*Tiktoken, error) {
	encoding, err := tiktoken.GetEncoding(encodingType)
	if err != nil {
		return nil, err
	}
	return &Tiktoken{
		encodingType: encodingType,
		encoding:     encoding,
	}, nil
}
