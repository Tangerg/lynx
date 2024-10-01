package json

import (
	"bufio"
	"fmt"
	"strings"
	"testing"
)

func TestJSONParser_Parse(t *testing.T) {
	jsonStr := `asdds]][]}}{}}{,}{"foo":"bar","arr":[1, 2, 3]},[1,2,3]]asdsd[1,["a","b","c"],{"foo":"bar"}] sdasdasd`
	parser := NewStreamParser(128)
	parser.OnObject = func(obj map[string]any) {
		t.Log(obj)
	}
	parser.OnArray = func(arr []any) {
		t.Log(arr)
	}
	err := parser.Parse(strings.NewReader(jsonStr))
	if err != nil {
		fmt.Printf("Failed to parse JSON: %v\n", err)
	}
	io := bufio.NewReadWriter(
		bufio.NewReader(strings.NewReader("")),
		bufio.NewWriter(nil),
	)

	for i := range jsonStr {
		_ = io.WriteByte(jsonStr[i])
		err = parser.Parse(io)
		if err != nil {
			fmt.Printf("Failed to parse JSON: %v\n", err)
		}
	}
}

func BenchmarkJSONParser_Parse(b *testing.B) {
	jsonStr := `asddsasadsad[{"foo": "bar", "arr": [1, 2, 3]}, [1,2,3]] asdasd [1,["a","b","c"],{"foo":"bar"}] sdasdasd`
	parser := NewStreamParser(128)
	reader := strings.NewReader(jsonStr)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.Parse(reader)
	}
	b.StopTimer()
}
