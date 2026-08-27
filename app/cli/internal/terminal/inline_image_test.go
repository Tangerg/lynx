package terminal

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/graphics"

	"github.com/Tangerg/scope/app/cli/internal/agent"
)

func TestInlineImageFallbackPreservesOutputIdentity(t *testing.T) {
	block := fallbackInlineImage(kit.Dark(), agent.InlineImage{
		Name: "chart.png", MIMEType: "image/png", Data: []byte("1234"),
	})
	picture, ok := block.(*kit.Image)
	if !ok || picture.Alt != "@ chart.png (image/png, 4 bytes)" {
		t.Fatalf("fallback image = %#v", block)
	}
}

type imageTransportStub struct {
	cell        image.Point
	transmitted int
}

func (*imageTransportStub) Protocol() graphics.Protocol { return graphics.Kitty }
func (i *imageTransportStub) CellSize() (image.Point, bool) {
	return i.cell, i.cell.X > 0 && i.cell.Y > 0
}
func (i *imageTransportStub) Transmit([]byte) (graphics.Image, error) {
	i.transmitted++
	return graphics.Image{ID: 1, Width: 200, Height: 100}, nil
}

func TestTerminalImageBlockReflowsWithTheHostCellSize(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 2, 1))
	var data bytes.Buffer
	if err := png.Encode(&data, source); err != nil {
		t.Fatal(err)
	}
	transport := &imageTransportStub{cell: image.Pt(10, 20)}
	presenter := newTerminalImagePresenter(transport)
	block := presenter.Present(kit.Dark(), agent.InlineImage{
		Name: "chart.png", MIMEType: "image/png", Data: data.Bytes(),
	})
	before := block.(*terminalImageBlock).Measure(40)
	transport.cell = image.Pt(20, 10)
	after := block.(*terminalImageBlock).Measure(40)
	if before == after || transport.transmitted != 1 {
		t.Fatalf("responsive image = rows %d -> %d, transmissions %d", before, after, transport.transmitted)
	}
}

func TestNormalizePNGConvertsSupportedImageFormats(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 2, 1))
	source.Set(0, 0, color.RGBA{R: 255, A: 255})
	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, source, nil); err != nil {
		t.Fatal(err)
	}
	converted, err := normalizePNG(jpegData.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(converted))
	if err != nil || config.Width != 2 || config.Height != 1 {
		t.Fatalf("converted PNG = config %+v, error %v", config, err)
	}
}
