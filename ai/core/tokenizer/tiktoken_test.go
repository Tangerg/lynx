package tokenizer

import (
	"testing"
)

func TestNewTiktoken(t *testing.T) {
	tik, err := NewTiktoken("o200k_base")
	t.Log(err)
	t.Log(tik)
}

func TestTiktoken(t *testing.T) {
	tik, _ := NewTiktoken("o200k_base")
	count, tokens := tik.EstimateTokens("hello,tiktoken")
	t.Log(count)
	for _, token := range tokens {
		t.Log(token)
	}
	str := tik.DecodeTokens(tokens)
	t.Log(str)

}
