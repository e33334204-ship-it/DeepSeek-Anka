package config

import (
	"regexp"
	"strings"
)

var validAPIKeyEnv = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// ValidAPIKeyEnvName reports whether s is a conventional environment variable name.
func ValidAPIKeyEnvName(s string) bool {
	return validAPIKeyEnv.MatchString(strings.TrimSpace(s))
}

// APIKeyEnvLooksLikeSecret reports when api_key_env was mistakenly set to the key
// itself (e.g. sk-...) instead of an env var name like MOONSHOT_API_KEY.
func APIKeyEnvLooksLikeSecret(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if ValidAPIKeyEnvName(s) {
		return false
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "sk-") || strings.HasPrefix(lower, "pk-") {
		return true
	}
	if len(s) > 48 {
		return true
	}
	return strings.Contains(s, "-")
}

// DefaultAPIKeyEnvForProvider guesses the env var name for a provider entry.
func DefaultAPIKeyEnvForProvider(name string) string {
	name = strings.TrimSpace(name)
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "kimi") && strings.Contains(lower, "coding"):
		return "KIMI_API_KEY"
	case strings.Contains(lower, "moonshot"):
		return "MOONSHOT_API_KEY"
	case strings.Contains(lower, "deepseek"):
		return "DEEPSEEK_API_KEY"
	case strings.Contains(lower, "openai"):
		return "OPENAI_API_KEY"
	case strings.Contains(lower, "anthropic"):
		return "ANTHROPIC_API_KEY"
	case strings.Contains(lower, "mimo"):
		return "MIMO_API_KEY"
	case strings.Contains(lower, "dashscope"), strings.Contains(lower, "qwen"):
		return "DASHSCOPE_API_KEY"
	case strings.Contains(lower, "minimax"):
		return "MINIMAX_API_KEY"
	case strings.Contains(lower, "ollama"):
		return "OLLAMA_API_KEY"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - 'a' + 'A')
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "API_KEY"
	}
	return out + "_API_KEY"
}
