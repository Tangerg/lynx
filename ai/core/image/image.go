package image

func New(url string, b64Json string) *Image {
	return &Image{url, b64Json}
}

type Image struct {
	url     string
	b64Json string
}

func (i *Image) Url() string {
	return i.url
}
func (i *Image) B64Json() string {
	return i.b64Json
}
