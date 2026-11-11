package problem

import (
	"bytes"
	"context"
	"image"
	_ "image/jpeg" // Only the standard PNG/JPEG decoders are admitted.
	"image/png"
	"path"
	"strings"
)

// Image limits bound admitted decode/encode work, not a hard CPU timeout.
const (
	MaxPreviewImages           = 16
	MaxPreviewImageBytes int64 = 256 << 10
	maxImagePixels       int64 = 4_000_000
	maxPreviewPixels     int64 = 8_000_000
)

// PreviewImage contains a re-encoded static PNG, never the source file's active
// content, animation, metadata or URL. Source is relative to the source task.md.
type PreviewImage struct {
	Source    string `json:"source"`
	MediaType string `json:"media_type"`
	Data      []byte `json:"data"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

func validImageSource(source string) bool {
	if !validPath(source) {
		return false
	}
	switch path.Ext(source) {
	case ".png", ".jpg", ".jpeg":
	default:
		return false
	}
	for _, part := range strings.Split(source, "/") {
		if part[0] == '.' {
			return false
		}
		for _, c := range part {
			switch {
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '-', c == '.':
			default:
				return false
			}
		}
	}
	return true
}

type imageBuffer struct{ bytes.Buffer }

func (b *imageBuffer) Write(p []byte) (int, error) {
	if int64(len(p)) > MaxPreviewImageBytes-int64(b.Len()) {
		return 0, ErrStore
	}
	return b.Buffer.Write(p)
}

func readPreviewImage(ctx context.Context, objects ReferenceObjectReader, file previewFile, pixels *int64) (PreviewImage, error) {
	raw, err := readPreviewBytes(ctx, objects, file.object)
	if err != nil {
		return PreviewImage{}, err
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	area := int64(config.Width) * int64(config.Height)
	if err != nil || (format != "png" && format != "jpeg") || config.Width < 1 || config.Height < 1 || config.Width > 4096 || config.Height > 4096 || area > maxImagePixels || area > maxPreviewPixels-*pixels {
		return PreviewImage{}, ErrStore
	}
	*pixels += area
	if err = ctx.Err(); err != nil {
		return PreviewImage{}, err
	}
	decoded, actualFormat, err := image.Decode(bytes.NewReader(raw))
	if err != nil || actualFormat != format || decoded.Bounds().Dx() != config.Width || decoded.Bounds().Dy() != config.Height {
		return PreviewImage{}, ErrStore
	}
	if err = ctx.Err(); err != nil {
		return PreviewImage{}, err
	}
	var out imageBuffer
	if err = png.Encode(&out, decoded); err != nil {
		return PreviewImage{}, ErrStore
	}
	if err = ctx.Err(); err != nil {
		return PreviewImage{}, err
	}
	return PreviewImage{MediaType: "image/png", Data: out.Bytes(), Width: config.Width, Height: config.Height}, nil
}
