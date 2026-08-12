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

	"github.com/Tangerg/lynx/app/cli/internal/agent"
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

func (presenter *terminalImagePresenter) Present(theme kit.Theme, image agent.InlineImage) headless.Block {
	fallback := fallbackInlineImage(theme, image)
	if presenter == nil || presenter.transport.Protocol() == graphics.None {
		return fallback
	}
	_, available := presenter.transport.CellSize()
	if !available {
		return fallback
	}
	key := sha256.Sum256(image.Data)
	handle, cached := presenter.cache[key]
	if !cached {
		data, err := normalizePNG(image.Data)
		if err != nil {
			return fallback
		}
		handle, err = presenter.transport.Transmit(data)
		if err != nil {
			return fallback
		}
		presenter.cache[key] = handle
	}
	return &terminalImageBlock{
		transport: presenter.transport, handle: handle, alt: inlineImageLabel(image), theme: theme,
	}
}

type terminalImageBlock struct {
	transport terminalImageTransport
	handle    graphics.Image
	alt       string
	theme     kit.Theme
}

func (block *terminalImageBlock) Measure(width int) int { return block.view().Measure(width) }

func (block *terminalImageBlock) Draw(view grid.View) { block.view().Draw(view) }

func (block *terminalImageBlock) view() kit.Image {
	cell, _ := block.transport.CellSize()
	return kit.Image{Of: block.handle, Cell: cell, Alt: block.alt, Theme: block.theme}
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
