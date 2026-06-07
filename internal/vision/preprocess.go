package vision

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// ImageInput is a normalized image block for vision analysis.
type ImageInput struct {
	Data     []byte
	MimeType string
	Index    int
}

// Resource is an image resource keyed by path (openhanako visual-context-pipeline).
type Resource struct {
	Key   string
	Label string
	Image ImageInput
}

// PreparedNote is a vision note returned from prepareResources.
type PreparedNote struct {
	Key    string
	Label  string
	Note   string
	Reused bool
}

const (
	maxVisionImageEdge   = 1600
	maxVisionUploadBytes = 900 * 1024
	visionJPEGQuality    = 86
)

// NormalizeModelImageInput loads and validates an image (model-image-preprocess.js).
func NormalizeModelImageInput(img ImageInput, index int) (ImageInput, error) {
	if len(img.Data) == 0 {
		return ImageInput{}, fmt.Errorf("image %d: empty data", index)
	}
	if len(img.Data) > 10*1024*1024 {
		return ImageInput{}, fmt.Errorf("image %d: exceeds 10 MB", index)
	}
	mime := img.MimeType
	if mime == "" {
		mime = detectMIME(img.Data, "")
	}
	if mime == "" {
		return ImageInput{}, fmt.Errorf("image %d: unsupported type", index)
	}
	data, optimizedMime := optimizeModelImage(img.Data, mime)
	return ImageInput{Data: data, MimeType: optimizedMime, Index: index}, nil
}

// LoadImageResource reads a local image file into a Resource. When workspaceRoot
// is set, repo-relative attachment paths (.deepseek-anka/attachments/...) resolve
// against it instead of the process cwd (desktop global tabs use a different root).
func LoadImageResource(workspaceRoot, key, path string) (Resource, error) {
	abs, err := resolveImagePath(workspaceRoot, path)
	if err != nil {
		return Resource{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Resource{}, err
	}
	if info.IsDir() || info.Size() <= 0 || info.Size() > 10*1024*1024 {
		return Resource{}, fmt.Errorf("image must be 1 byte to 10 MB")
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return Resource{}, err
	}
	mime := detectMIME(raw, abs)
	if mime == "" {
		return Resource{}, fmt.Errorf("unsupported image type")
	}
	useKey := key
	if useKey == "" {
		useKey = filepath.ToSlash(abs)
	}
	return Resource{
		Key:   useKey,
		Label: filepath.Base(abs),
		Image: ImageInput{Data: raw, MimeType: mime},
	}, nil
}

// ResolveAttachmentPath resolves a repo-relative attachment path against workspaceRoot,
// falling back to the process cwd (openhanako uses absolute paths; Anka attachments
// live under .deepseek-anka/attachments relative to the tab workspace).
func ResolveAttachmentPath(workspaceRoot, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("image path is empty")
	}
	slash := filepath.ToSlash(path)
	if strings.HasPrefix(slash, ".deepseek-anka/attachments/") || strings.HasPrefix(slash, ".reasonix/attachments/") {
		root := strings.TrimSpace(workspaceRoot)
		if root != "" {
			candidate := filepath.Join(root, filepath.FromSlash(slash))
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
		return filepath.Abs(path)
	}
	return filepath.Abs(path)
}

func resolveImagePath(workspaceRoot, path string) (string, error) {
	return ResolveAttachmentPath(workspaceRoot, path)
}

func detectMIME(raw []byte, path string) string {
	if len(raw) >= 8 && raw[0] == 0x89 && raw[1] == 'P' {
		return "image/png"
	}
	if len(raw) >= 3 && raw[0] == 0xFF && raw[1] == 0xD8 {
		return "image/jpeg"
	}
	if len(raw) >= 6 && (string(raw[:6]) == "GIF87a" || string(raw[:6]) == "GIF89a") {
		return "image/gif"
	}
	if len(raw) >= 12 && string(raw[0:4]) == "RIFF" && string(raw[8:12]) == "WEBP" {
		return "image/webp"
	}
	switch filepath.Ext(path) {
	case ".png", ".PNG":
		return "image/png"
	case ".jpg", ".jpeg", ".JPG", ".JPEG":
		return "image/jpeg"
	case ".gif", ".GIF":
		return "image/gif"
	case ".webp", ".WEBP":
		return "image/webp"
	}
	return ""
}

func optimizeModelImage(raw []byte, mime string) ([]byte, string) {
	if mime != "image/png" && mime != "image/jpeg" {
		return raw, mime
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return raw, mime
	}
	needsResize := cfg.Width > maxVisionImageEdge || cfg.Height > maxVisionImageEdge
	if !needsResize && len(raw) <= maxVisionUploadBytes {
		return raw, mime
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return raw, mime
	}
	rgba := flattenOnWhite(src)
	if needsResize {
		rgba = resizeNearest(rgba, maxVisionImageEdge)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, rgba, &jpeg.Options{Quality: visionJPEGQuality}); err != nil || buf.Len() == 0 {
		return raw, mime
	}
	if !needsResize && buf.Len() >= len(raw) {
		return raw, mime
	}
	return buf.Bytes(), "image/jpeg"
}

func flattenOnWhite(src image.Image) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Over)
	return dst
}

func resizeNearest(src image.Image, maxEdge int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if maxEdge <= 0 || w <= 0 || h <= 0 || (w <= maxEdge && h <= maxEdge) {
		if rgba, ok := src.(*image.RGBA); ok {
			return rgba
		}
		return flattenOnWhite(src)
	}
	scale := float64(maxEdge) / float64(max(w, h))
	nw := max(1, int(math.Round(float64(w)*scale)))
	nh := max(1, int(math.Round(float64(h)*scale)))
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		sy := b.Min.Y + min(h-1, int(float64(y)*float64(h)/float64(nh)))
		for x := 0; x < nw; x++ {
			sx := b.Min.X + min(w-1, int(float64(x)*float64(w)/float64(nw)))
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}
