package vision

import (
	"net/url"
	"strings"
)

// normalizeOpenAICompatBaseURL ensures Moonshot/Kimi OpenAI-compatible endpoints
// include the /v1 suffix required by https://platform.kimi.com/docs/api/overview
func normalizeOpenAICompatBaseURL(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if base == "" {
		return base
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return base
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "api.moonshot.cn" || host == "api.moonshot.ai" {
		if !strings.HasSuffix(base, "/v1") {
			return base + "/v1"
		}
	}
	return base
}
