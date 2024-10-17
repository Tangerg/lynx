package mime

var (
	All         = newMime(wildcardType, wildcardType)
	Text        = newMime("text", wildcardType)
	Video       = newMime("video", wildcardType)
	Audio       = newMime("audio", wildcardType)
	Image       = newMime("image", wildcardType)
	Application = newMime("application", wildcardType)
	Multipart   = newMime("multipart", wildcardType)
	Font        = newMime("font", wildcardType)
	Message     = newMime("message", wildcardType)
	Model       = newMime("model", wildcardType)
	Chemical    = newMime("chemical", wildcardType)
	Example     = newMime("example", wildcardType)
)

var (
	extToMimeType = map[string]*Mime{}
)

func init() {
	for ext, mimeTypeStr := range extToMimeTypeString {
		extToMimeType[ext], _ = Parse(mimeTypeStr)
	}
}

func TypeByExtension(ext string) (*Mime, bool) {
	mimt, ok := extToMimeType[ext]
	if ok {
		return mimt.Clone(), ok
	}
	return nil, false
}
