package tokenizer

type Tokenizer interface {
	EncodingType() string
	Estimate(text string) int
	EstimateTokens(text string) (int, []int)
	EncodeTokens(text string) []int
	DecodeTokens(tokens []int) string
}
