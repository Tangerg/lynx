package mime

import (
	"testing"
)

func TestNormalizeXSubtype(t *testing.T) {
	md, _ := Parse("text/x-markdown;charset=utf-8")
	normalizeXSubtype := NormalizeXSubtype(md)
	t.Log(normalizeXSubtype.String())

	md, _ = Parse("video/x-mp4; codecs=\"avc1.64001E, mp4a.40.2\"; width=1920; height=1080 ")
	normalizeXSubtype = NormalizeXSubtype(md)
	t.Log(normalizeXSubtype.String())
}
