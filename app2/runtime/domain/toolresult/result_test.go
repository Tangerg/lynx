package toolresult

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestProjectBoundsLargeUTF8BodyWithStableReadHandle(t *testing.T) {
	body := strings.Repeat("界", 30_000)
	first := Project("itm_exact", body)
	second := Project("itm_exact", body)
	if !first.Offloaded || first.ID == "" || first.ID != second.ID || first.Preview != second.Preview {
		t.Fatalf("large projection = %+v, repeat = %+v", first, second)
	}
	if len(first.Preview) >= len(body) || !utf8.ValidString(first.Preview) ||
		!strings.Contains(first.Preview, `"result_id":"`+first.ID+`"`) {
		t.Fatalf("invalid bounded preview (%d/%d bytes): %q", len(first.Preview), len(body), first.Preview)
	}
	inline := Project("itm_small", `{"ok":true}`)
	if inline.Offloaded || inline.ID != "" || inline.Preview != `{"ok":true}` {
		t.Fatalf("inline projection = %+v", inline)
	}
}
