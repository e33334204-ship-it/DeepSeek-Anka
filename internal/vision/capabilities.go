package vision

import (
	"net/url"
	"strings"

	"deepseek-anka/internal/config"
)

// VisionCapabilities describes coordinate grounding for structured vision analysis.
type VisionCapabilities struct {
	Grounding       bool
	Boxes           bool
	Points          bool
	CoordinateSpace string
	BoxOrder        string
	OutputFormat    string
	GroundingMode   string
}

// InferModelInput approximates openhanako model.input for Reasonix provider entries.
func InferModelInput(entry *config.ProviderEntry, modelID string) []string {
	if entry == nil {
		return []string{"text"}
	}
	if isOfficialDeepSeekEndpoint(entry) {
		return []string{"text"}
	}
	if modelLikelySupportsImage(modelID) || modelLikelySupportsImage(entry.Name) {
		return []string{"text", "image"}
	}
	return []string{"text"}
}

// RequiresAuxiliaryVision is true when the chat model cannot accept images directly
// (openhanako requiresAuxiliaryVision — used once images are present).
func RequiresAuxiliaryVision(target TargetModel) bool {
	entry := target.Entry
	modelID := target.Ref.ID
	if modelID == "" && entry != nil {
		modelID = entry.Model
	}
	return !ModelSupportsDirectImageInput(entry, modelID)
}

// ModelSupportsDirectImageInput mirrors shared/model-capabilities.js.
func ModelSupportsDirectImageInput(entry *config.ProviderEntry, modelID string) bool {
	if isOfficialDeepSeekEndpoint(entry) || isMoonshotOrKimiEndpoint(entry) {
		return false
	}
	if !containsInput(InferModelInput(entry, modelID), "image") {
		return false
	}
	return true
}

// ModelSupportsImage reports whether a model can be used as the auxiliary vision model.
func ModelSupportsImage(entry *config.ProviderEntry, modelID string) bool {
	if isOfficialDeepSeekEndpoint(entry) {
		return false
	}
	return containsInput(InferModelInput(entry, modelID), "image")
}

// GetVisionCapabilities resolves grounding metadata for structured primitive output.
func GetVisionCapabilities(entry *config.ProviderEntry, modelID string) *VisionCapabilities {
	id := strings.ToLower(modelID)
	if strings.Contains(id, "gemini") {
		return &VisionCapabilities{
			Grounding: true, Boxes: true, BoxOrder: "yxyx",
			CoordinateSpace: "norm-1000", OutputFormat: "gemini", GroundingMode: "native",
		}
	}
	if strings.Contains(id, "qwen") && strings.Contains(id, "vl") {
		return &VisionCapabilities{
			Grounding: true, Boxes: true, Points: true, BoxOrder: "xyxy",
			CoordinateSpace: "norm-1000", OutputFormat: "qwen", GroundingMode: "native",
		}
	}
	if strings.Contains(id, "cua") || strings.Contains(id, "computer") {
		return &VisionCapabilities{
			Grounding: true, Boxes: true, Points: true, BoxOrder: "xyxy",
			CoordinateSpace: "norm-1000", OutputFormat: "anchor", GroundingMode: "native",
		}
	}
	return nil
}

func TargetModelFromConfig(cfg *config.Config) TargetModel {
	if cfg == nil {
		return TargetModel{}
	}
	entry, ok := cfg.ResolveModel(cfg.DefaultModel)
	if !ok {
		return TargetModel{}
	}
	return targetFromEntry(entry)
}

func containsInput(input []string, kind string) bool {
	for _, v := range input {
		if v == kind {
			return true
		}
	}
	return false
}

func modelLikelySupportsImage(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return false
	}
	markers := []string{
		"gpt-4o", "gpt-4-turbo", "gpt-4.1", "gpt-4.5",
		"claude-3", "claude-sonnet-4", "claude-opus-4",
		"gemini", "qwen-vl", "qwen2-vl", "qwen3-vl",
		"pixtral", "llava", "glm-4v", "glm-5v", "yi-vision",
		"gpt-4-vision", "vision", "-vl", "_vl",
		"kimi-k2", "kimi-latest", "kimi-for-coding",
		"doubao-vision", "doubao-seed", "grok",
		"llama-4", "internvl", "step-1v", "hunyuan-vision",
		"mimo-v2-omni", "minimax-m3",
	}
	for _, m := range markers {
		if strings.Contains(n, m) {
			return true
		}
	}
	return false
}

func isOfficialDeepSeekEndpoint(entry *config.ProviderEntry) bool {
	if entry == nil {
		return false
	}
	if strings.EqualFold(entry.Name, "deepseek") {
		return true
	}
	host := baseHost(entry.BaseURL)
	return host == "api.deepseek.com"
}

// isMoonshotOrKimiEndpoint reports Moonshot platform / Kimi Coding hosts where
// chat models are text-only and images must go through auxiliary vision (openhanako).
func isMoonshotOrKimiEndpoint(entry *config.ProviderEntry) bool {
	if entry == nil {
		return false
	}
	host := baseHost(entry.BaseURL)
	if host == "api.moonshot.cn" || host == "api.moonshot.ai" {
		return true
	}
	return strings.Contains(host, "kimi.com")
}

func baseHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return strings.ToLower(strings.Split(strings.TrimPrefix(raw, "https://"), "/")[0])
	}
	return strings.ToLower(u.Hostname())
}
