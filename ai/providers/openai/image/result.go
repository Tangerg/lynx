package image

import "github.com/Tangerg/lynx/ai/core/image/result"

var _ result.ImageResultMetadata = (*OpenAIImageResultMetadata)(nil)

type OpenAIImageResultMetadata struct {
}
