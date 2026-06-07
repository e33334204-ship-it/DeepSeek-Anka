package main

import (
	"testing"

	"deepseek-anka/internal/config"
	"deepseek-anka/internal/provider"
)

func TestWithFreshSystemPromptReplacesExistingSystemMessage(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleSystem, Content: "old", ReasoningContent: "stale", ReasoningSignature: "sig", ToolCalls: []provider.ToolCall{{ID: "call", Name: "noop"}}, ToolCallID: "tool", Name: "name"},
		{Role: provider.RoleUser, Content: "hello"},
	}

	got := withFreshSystemPrompt(msgs, "new")
	if got[0].Content != "new" {
		t.Fatalf("system prompt = %q, want new", got[0].Content)
	}
	if got[0].ReasoningContent != "" || got[0].ReasoningSignature != "" || len(got[0].ToolCalls) != 0 || got[0].ToolCallID != "" || got[0].Name != "" {
		t.Fatalf("system metadata should be cleared, got %+v", got[0])
	}
	if got[1].Content != "hello" {
		t.Fatalf("non-system message changed: %+v", got[1])
	}
	if msgs[0].Content != "old" {
		t.Fatalf("input slice was mutated: %+v", msgs[0])
	}
}

func TestWithFreshSystemPromptPrependsMissingSystemMessage(t *testing.T) {
	msgs := []provider.Message{{Role: provider.RoleUser, Content: "hello"}}

	got := withFreshSystemPrompt(msgs, "new")
	if len(got) != 2 || got[0].Role != provider.RoleSystem || got[0].Content != "new" {
		t.Fatalf("expected prepended system prompt, got %+v", got)
	}
	if got[1].Content != "hello" {
		t.Fatalf("existing user message changed: %+v", got[1])
	}
}

func TestRepairBrokenVisionRefUsesConfiguredProviderForSameModel(t *testing.T) {
	cfg := &config.Config{
		Vision: config.VisionConfig{Enabled: true, Model: "kimi-coding-vision/kimi-k2.6"},
		Providers: []config.ProviderEntry{{
			Name:      "moonshot-vision",
			Kind:      "openai",
			BaseURL:   "https://api.moonshot.cn/v1",
			Models:    []string{"kimi-k2.6", "kimi-k2.5"},
			Default:   "kimi-k2.5",
			APIKeyEnv: "MOONSHOT_API_KEY",
		}},
	}

	if !repairBrokenVisionRef(cfg) {
		t.Fatal("repairBrokenVisionRef returned false")
	}
	if cfg.Vision.Model != "moonshot-vision/kimi-k2.6" {
		t.Fatalf("vision model = %q, want moonshot-vision/kimi-k2.6", cfg.Vision.Model)
	}
}
