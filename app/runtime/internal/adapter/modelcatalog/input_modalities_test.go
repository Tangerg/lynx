package modelcatalog

import (
	"errors"
	"testing"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/media"

	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
)

func TestCapabilitiesAdmitInputUsesExactCatalogModel(t *testing.T) {
	image, err := media.NewBytes("image/png", []byte("image"))
	if err != nil {
		t.Fatal(err)
	}
	messages := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("inspect"), chat.NewMediaPart(image)),
	}
	textOnly, err := modelref.New("alibaba", "qwen-mt-plus")
	if err != nil {
		t.Fatal(err)
	}
	if admitErr := (Capabilities{}).AdmitInput(textOnly, messages); !errors.Is(admitErr, ErrUnsupportedInputModality) {
		t.Fatalf("text-only model error = %v, want ErrUnsupportedInputModality", admitErr)
	}

	imageCapable, err := modelref.New("openai", "gpt-5.6-sol")
	if err != nil {
		t.Fatal(err)
	}
	if admitErr := (Capabilities{}).AdmitInput(imageCapable, messages); admitErr != nil {
		t.Fatalf("image-capable model error = %v", admitErr)
	}
}

func TestCapabilitiesAdmitInputAllowsPrivateCatalogMiss(t *testing.T) {
	image, err := media.NewBytes("image/png", []byte("image"))
	if err != nil {
		t.Fatal(err)
	}
	selection, err := modelref.New("openai-compatible", "private-vision-model")
	if err != nil {
		t.Fatal(err)
	}
	if err := (Capabilities{}).AdmitInput(selection, []chat.Message{
		chat.NewUserMessage(chat.NewMediaPart(image)),
	}); err != nil {
		t.Fatalf("private model error = %v", err)
	}
}

func TestCatalogInputModalityClassifiesOnlyModelContent(t *testing.T) {
	text, relevant, err := catalogInputModality(chat.NewTextPart("hello"))
	if err != nil || !relevant || text != "text" {
		t.Fatalf("text modality = (%q, %t, %v), want (text, true, nil)", text, relevant, err)
	}
	_, relevant, err = catalogInputModality(chat.NewReasoningPart("thinking", nil))
	if err != nil || relevant {
		t.Fatalf("reasoning modality = (_, %t, %v), want (_, false, nil)", relevant, err)
	}
}

func TestCatalogInputModalityRejectsUnsupportedMediaType(t *testing.T) {
	archive, err := media.NewBytes("application/zip", []byte("archive"))
	if err != nil {
		t.Fatal(err)
	}
	_, relevant, err := catalogInputModality(chat.NewMediaPart(archive))
	if !relevant || err == nil {
		t.Fatalf("archive modality = (_, %t, %v), want relevant error", relevant, err)
	}
}
