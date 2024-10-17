package mime

var (
	All         = newMime(wildcardType, wildcardType)
	Text        = newMime("text", "*")
	Video       = newMime("video", "*")
	Audio       = newMime("audio", "*")
	Image       = newMime("image", "*")
	Application = newMime("application", "*")
	Multipart   = newMime("multipart", "*")
	Font        = newMime("font", "*")
	Message     = newMime("message", "*")
	Model       = newMime("model", "*")
	Chemical    = newMime("chemical", "*")
	Example     = newMime("example", "*")
)
