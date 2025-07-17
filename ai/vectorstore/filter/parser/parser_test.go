package parser

import (
	"testing"
)

func TestParser_Parse(t *testing.T) {
	parser, err := NewParser("name = 'Tom' AND NOT (age >= 18 or age < -15) and status in ('pending','completed') and score in (1,2,3)")
	if err != nil {
		t.Fatal(err)
	}
	parse, err := parser.Parse()
	if err != nil {
		t.Fatal(err)
	}
	t.Log(parse.String())
}
