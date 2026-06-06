package config

import "testing"

func TestResolveVisionModelRequiresExplicitModel(t *testing.T) {
	cfg := Default()
	cfg.Vision.Enabled = true
	cfg.Vision.Model = ""
	if _, err := cfg.ResolveVisionModel(); err == nil {
		t.Fatal("expected error when vision enabled without model")
	}
}

func TestResolveVisionModelFindsProvider(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	cfg := Default()
	cfg.Providers = append(cfg.Providers, ProviderEntry{
		Name: "gpt-4o", Kind: "openai", BaseURL: "https://api.openai.com/v1",
		Model: "gpt-4o", APIKeyEnv: "OPENAI_API_KEY",
	})
	cfg.Vision.Enabled = true
	cfg.Vision.Model = "gpt-4o"
	entry, err := cfg.ResolveVisionModel()
	if err != nil {
		t.Fatal(err)
	}
	if entry == nil || entry.Name != "gpt-4o" {
		t.Fatalf("got %+v", entry)
	}
}
