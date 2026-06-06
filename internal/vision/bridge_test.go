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

func TestUniqueImagePathsFromText(t *testing.T) {
	text := "see @.deepseek-anka/attachments/a.png and [attached_image: /tmp/b.jpg]"
	paths := UniqueImagePathsFromText(text)
	if len(paths) != 2 {
		t.Fatalf("got %d paths, want 2: %v", len(paths), paths)
	}
}

func TestFormatStructuredVisionNote(t *testing.T) {
	note := formatStructuredVisionNote(map[string]any{
		"image_overview":       "A login form",
		"visible_text":         []any{"Sign in"},
		"objects_and_layout":   "button bottom-right",
		"charts_or_data":       "none",
		"user_request":         "find submit",
		"user_request_answer":  "bottom-right blue button",
		"evidence":             "visible label Submit",
		"uncertainty":          "none",
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
