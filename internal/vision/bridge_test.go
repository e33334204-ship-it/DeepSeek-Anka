package vision

import (
	"testing"

	"deepseek-anka/internal/config"
)

func TestRequiresAuxiliaryVisionDeepSeek(t *testing.T) {
	entry := &config.ProviderEntry{
		Name: "deepseek", Kind: "openai",
		BaseURL: "https://api.deepseek.com", Model: "deepseek-chat",
	}
	target := TargetModel{Ref: ModelRef{ID: "deepseek-chat", Provider: "deepseek"}, Entry: entry}
	if !RequiresAuxiliaryVision(target) {
		t.Fatal("deepseek chat should require auxiliary vision when image input is requested")
	}
}

func TestModelSupportsDirectImageGPT4o(t *testing.T) {
	entry := &config.ProviderEntry{
		Name: "gpt-4o", Kind: "openai",
		BaseURL: "https://api.openai.com/v1", Model: "gpt-4o",
	}
	if !ModelSupportsDirectImageInput(entry, "gpt-4o") {
		t.Fatal("gpt-4o should support direct image input")
	}
	if RequiresAuxiliaryVision(TargetModel{Ref: ModelRef{ID: "gpt-4o", Provider: "gpt-4o"}, Entry: entry}) {
		t.Fatal("gpt-4o should not require auxiliary vision")
	}
}

func TestMoonshotKimiCanBeAuxiliaryVisionModel(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		model   string
	}{
		{name: "moonshot", baseURL: "https://api.moonshot.cn/v1", model: "kimi-k2.6"},
		{name: "kimi coding", baseURL: "https://api.kimi.com/coding/v1", model: "kimi-for-coding"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &config.ProviderEntry{
				Name: tt.name, Kind: "openai", BaseURL: tt.baseURL, Model: tt.model,
			}
			if ModelSupportsDirectImageInput(entry, tt.model) {
				t.Fatal("Moonshot/Kimi chat endpoints should still route image input through auxiliary vision")
			}
			if !RequiresAuxiliaryVision(TargetModel{Ref: ModelRef{ID: tt.model, Provider: tt.name}, Entry: entry}) {
				t.Fatal("Moonshot/Kimi target chat model should require auxiliary vision")
			}
			if !ModelSupportsImage(entry, tt.model) {
				t.Fatal("Moonshot/Kimi vision-capable models should be accepted as auxiliary vision models")
			}
		})
	}
}

func TestOfficialDeepSeekCannotBeAuxiliaryVisionModel(t *testing.T) {
	entry := &config.ProviderEntry{
		Name: "deepseek", Kind: "openai",
		BaseURL: "https://api.deepseek.com", Model: "deepseek-chat",
	}
	if ModelSupportsImage(entry, "deepseek-chat") {
		t.Fatal("official DeepSeek endpoint must not be accepted as auxiliary vision")
	}
}

func TestUniqueImagePathsFromText(t *testing.T) {
	text := "see @.deepseek-anka/attachments/a.png and [attached_image: /tmp/b.jpg]"
	paths := UniqueImagePathsFromText(text)
	if len(paths) != 2 {
		t.Fatalf("got %d paths, want 2: %v", len(paths), paths)
	}
}

func TestStripImageRefsFromText(t *testing.T) {
	text := "@.deepseek-anka/attachments/a.png\n描述一下这张图片\n[attached_image: /tmp/b.jpg]"
	got := StripImageRefsFromText(text)
	if got != "描述一下这张图片" {
		t.Fatalf("StripImageRefsFromText = %q", got)
	}
}

func TestFormatStructuredVisionNote(t *testing.T) {
	note := formatStructuredVisionNote(map[string]any{
		"image_overview":      "A login form",
		"visible_text":        []any{"Sign in"},
		"objects_and_layout":  "button bottom-right",
		"charts_or_data":      "none",
		"user_request":        "find submit",
		"user_request_answer": "bottom-right blue button",
		"evidence":            "visible label Submit",
		"uncertainty":         "none",
	}, nil)
	if note == "" || !contains(note, "image_overview:") {
		t.Fatalf("unexpected note: %q", note)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
