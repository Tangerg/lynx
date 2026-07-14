package bedrock

import (
	"mime"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/Tangerg/lynx/core/media"
)

func toDocument(value any) document.Interface {
	if value == nil {
		return nil
	}
	if object, ok := value.(map[string]any); ok && len(object) == 0 {
		return nil
	}
	return document.NewLazyDocument(value)
}

func mediaToBlock(value *media.Media) types.ContentBlock {
	if value == nil {
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(value.MIME)
	if err != nil {
		return nil
	}
	major, subtype, ok := strings.Cut(mediaType, "/")
	if !ok || !strings.EqualFold(major, "image") {
		return nil
	}
	format, ok := bedrockImageFormat(subtype)
	if !ok {
		return nil
	}
	raw, err := value.Bytes()
	if err != nil || len(raw) == 0 {
		return nil
	}
	return &types.ContentBlockMemberImage{
		Value: types.ImageBlock{
			Format: format,
			Source: &types.ImageSourceMemberBytes{Value: raw},
		},
	}
}

func bedrockImageFormat(subtype string) (types.ImageFormat, bool) {
	switch strings.ToLower(subtype) {
	case "png":
		return types.ImageFormatPng, true
	case "jpeg", "jpg":
		return types.ImageFormatJpeg, true
	case "gif":
		return types.ImageFormatGif, true
	case "webp":
		return types.ImageFormatWebp, true
	default:
		return "", false
	}
}
