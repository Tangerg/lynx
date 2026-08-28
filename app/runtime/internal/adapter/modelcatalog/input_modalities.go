package modelcatalog

import (
	"errors"
	"fmt"
	"mime"
	"strings"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/media"
	"github.com/Tangerg/scope/models/catalog"

	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
)

var ErrUnsupportedInputModality = errors.New("modelcatalog: unsupported input modality")

// AdmitInput rejects media that a known catalog model does not
// accept. A catalog miss remains admissible because compatible endpoints may
// expose private models whose capabilities are not available locally.
func (Capabilities) AdmitInput(selection modelref.Selection, messages []chat.Message) error {
	if err := selection.Validate(); err != nil {
		return fmt.Errorf("modelcatalog: input-modality selection: %w", err)
	}
	entry, found := catalog.Default.Lookup(selection.Provider(), selection.Model())
	if !found {
		return nil
	}
	for messageIndex := range messages {
		for partIndex := range messages[messageIndex].Parts {
			part := messages[messageIndex].Parts[partIndex]
			modality, relevant, err := catalogInputModality(part)
			if !relevant {
				continue
			}
			if err != nil {
				return fmt.Errorf(
					"%w: model %q/%q message %d part %d: %w",
					ErrUnsupportedInputModality,
					selection.Provider(),
					selection.Model(),
					messageIndex,
					partIndex,
					err,
				)
			}
			if !entry.Modalities.AcceptsInput(modality) {
				return fmt.Errorf(
					"%w: model %q/%q does not accept %s input",
					ErrUnsupportedInputModality,
					selection.Provider(),
					selection.Model(),
					modality,
				)
			}
		}
	}
	return nil
}

func catalogInputModality(part chat.Part) (catalog.Modality, bool, error) {
	switch part.Kind {
	case chat.PartText:
		return catalog.ModalityText, true, nil
	case chat.PartMedia:
		modality, err := catalogMediaInputModality(part.Media)
		return modality, true, err
	default:
		return "", false, nil
	}
}

func catalogMediaInputModality(value *media.Media) (catalog.Modality, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	mediaType, _, _ := mime.ParseMediaType(value.MIME)
	major, _, _ := strings.Cut(mediaType, "/")
	switch major {
	case "image":
		return catalog.ModalityImage, nil
	case "audio":
		return catalog.ModalityAudio, nil
	case "video":
		return catalog.ModalityVideo, nil
	case "application":
		if mediaType == "application/pdf" {
			return catalog.ModalityPDF, nil
		}
	}
	return "", fmt.Errorf("media type %q has no supported chat input modality", mediaType)
}
