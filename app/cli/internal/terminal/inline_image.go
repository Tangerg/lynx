package terminal

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/graphics"
	"github.com/Tangerg/oolong/core/grid"

	"github.com/Tangerg/scope/app/cli/internal/agent"
)

type terminalImagePresenter struct {
	transport terminalImageTransport
	cache     map[[sha256.Size]byte]graphics.Image
}

type terminalImageTransport interface {
	Protocol() graphics.Protocol
	CellSize() (image.Point, bool)
	Transmit([]byte) (graphics.Image, error)
}

func newTerminalImagePresenter(transport terminalImageTransport) *terminalImagePresenter {
	return &terminalImagePresenter{transport: transport, cache: make(map[[sha256.Size]byte]graphics.Image)}
}

func (t *terminalImagePresenter) Present(theme kit.Theme, image agent.InlineImage) headless.Block {
	fallback := fallbackInlineImage(theme, image)
	if t == nil || t.transport.Protocol() == graphics.None {
		return fallback
	}
	_, available := t.transport.CellSize()
	if !available {
		return fallback
	}
	key := sha256.Sum256(image.Data)
	handle, cached := t.cache[key]
	if !cached {
		data, err := normalizePNG(image.Data)
		if err != nil {
			return fallback
		}
		handle, err = t.transport.Transmit(data)
		if err != nil {
			return fallback
		}
		t.cache[key] = handle
	}
	return &terminalImageBlock{
		transport: t.transport, handle: handle, alt: inlineImageLabel(image), theme: theme,
	}
}

type terminalImageBlock struct {
	transport terminalImageTransport
	handle    graphics.Image
	alt       string
	theme     kit.Theme
}

func (t *terminalImageBlock) Measure(width int) int { return t.view().Measure(width) }

func (t *terminalImageBlock) Draw(view grid.View) { t.view().Draw(view) }

func (t *terminalImageBlock) view() kit.Image {
	cell, _ := t.transport.CellSize()
	return kit.Image{Of: t.handle, Cell: cell, Alt: t.alt, Theme: t.theme}
}

func fallbackInlineImage(theme kit.Theme, image agent.InlineImage) headless.Block {
	return &kit.Image{Alt: inlineImageLabel(image), Theme: theme}
}

func inlineImageLabel(image agent.InlineImage) string {
	return fmt.Sprintf("@ %s (%s, %d bytes)", image.Name, image.MIMEType, len(image.Data))
}

func normalizePNG(data []byte) ([]byte, error) {
	if _, err := png.DecodeConfig(bytes.NewReader(data)); err == nil {
		return data, nil
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode inline image: %w", err)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, decoded); err != nil {
		return nil, fmt.Errorf("encode inline image as PNG: %w", err)
	}
	return encoded.Bytes(), nil
}
