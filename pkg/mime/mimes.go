package mime

var (
	All, _         = newMime(wildcardType, wildcardType)
	Text, _        = newMime("text", "*")
	Video, _       = newMime("video", "*")
	Audio, _       = newMime("audio", "*")
	Image, _       = newMime("image", "*")
	Application, _ = newMime("application", "*")
	Multipart, _   = newMime("multipart", "*")
	Font, _        = newMime("font", "*")
	Message, _     = newMime("message", "*")
	Model, _       = newMime("model", "*")
	Chemical, _    = newMime("chemical", "*")
	Example, _     = newMime("example", "*")
)
