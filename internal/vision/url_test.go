package vision

import "testing"

func TestNormalizeOpenAICompatBaseURL(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"https://api.moonshot.cn", "https://api.moonshot.cn/v1"},
		{"https://api.moonshot.cn/", "https://api.moonshot.cn/v1"},
		{"https://api.moonshot.cn/v1", "https://api.moonshot.cn/v1"},
		{"https://api.openai.com/v1", "https://api.openai.com/v1"},
	}
	for _, tc := range tests {
		if got := normalizeOpenAICompatBaseURL(tc.in); got != tc.want {
			t.Fatalf("normalizeOpenAICompatBaseURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
