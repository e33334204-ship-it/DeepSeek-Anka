package vision

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestNormalizeModelImageInputOptimizesLargePNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2600, 1800))
	for y := 0; y < 1800; y++ {
		for x := 0; x < 2600; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8((x + y) % 251),
				G: uint8((x * 3) % 251),
				B: uint8((y * 5) % 251),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	got, err := NormalizeModelImageInput(ImageInput{Data: buf.Bytes(), MimeType: "image/png"}, 0)
	if err != nil {
		t.Fatalf("NormalizeModelImageInput returned error: %v", err)
	}
	if got.MimeType != "image/jpeg" {
		t.Fatalf("MimeType = %q, want image/jpeg", got.MimeType)
	}
	if len(got.Data) > maxVisionUploadBytes {
		t.Fatalf("optimized image still exceeds upload target: got %d max %d", len(got.Data), maxVisionUploadBytes)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(got.Data))
	if err != nil {
		t.Fatalf("decode optimized image: %v", err)
	}
	if cfg.Width > maxVisionImageEdge || cfg.Height > maxVisionImageEdge {
		t.Fatalf("optimized dimensions = %dx%d, max edge %d", cfg.Width, cfg.Height, maxVisionImageEdge)
	}
}
