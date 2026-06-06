package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"deepseek-anka/internal/capabilities"
	"deepseek-anka/internal/control"
	"deepseek-anka/internal/tool"
	"deepseek-anka/internal/vision"
)

func init() { tool.RegisterBuiltin(understandImage{}) }

type understandImage struct{}

func (understandImage) Name() string { return "understand_image" }

func (understandImage) Description() string {
	return "Analyze a local image file and return a detailed textual description. Use when the user attaches an image or when visual understanding is needed for screenshots and browser captures."
}

func (understandImage) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "path":{"type":"string","description":"Path to a local image file (e.g. .deepseek-anka/attachments/... or an absolute path)"},
  "context":{"type":"string","description":"Optional user question or task context for the analysis"}
},
"required":["path"]
}`)
}

func (understandImage) ReadOnly() bool { return true }

func (understandImage) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Path    string `json:"path"`
		Context string `json:"context"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if strings.TrimSpace(p.Path) == "" {
		return "", fmt.Errorf("path is required")
	}
	path := p.Path
	if strings.HasPrefix(filepath.ToSlash(path), ".deepseek-anka/attachments/") {
		clean, err := control.CleanAttachmentPath(path)
		if err != nil {
			return "", err
		}
		path = clean
	}
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	br := capabilities.VisionBridge()
	if br == nil {
		return "", fmt.Errorf("vision is disabled; enable [vision] in reasonix.toml and configure a vision-capable model")
	}
	resource, err := vision.LoadImageResource(path, path)
	if err != nil {
		return "", err
	}
	notes, err := br.SummarizeResources(ctx, vision.ResourcesOptions{
		UserRequest: p.Context,
		Resources:   []vision.Resource{resource},
	})
	if err != nil {
		return "", err
	}
	if len(notes) == 0 {
		return "", fmt.Errorf("vision analysis produced no note")
	}
	return vision.WrapNote(notes[0].Note), nil
}
