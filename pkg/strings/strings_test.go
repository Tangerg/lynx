package strings

import "testing"

func TestIsQuotedString(t *testing.T) {
	t.Log(IsQuoted("asd"))
	t.Log(IsQuoted(`"asd"`))
	t.Log(IsQuoted("a"))
	t.Log(IsQuoted(`""`))
	t.Log(IsQuoted("\"a"))
	t.Log(IsQuoted("\"a\""))
	t.Log(IsQuoted("'"))
	t.Log(IsQuoted("''"))
	t.Log(IsQuoted("'asd'"))
}

func TestUnQuote(t *testing.T) {
	t.Log(UnQuote(`"asd"`))
	t.Log(UnQuote(`"asdsads"`))
	t.Log(UnQuote("'asd'"))
	t.Log(UnQuote("'asd"))
	t.Log(UnQuote("\"asd"))
	t.Log(UnQuote("\"\""))
}
