package config

import "testing"

func TestValidAPIKeyEnvName(t *testing.T) {
	if !ValidAPIKeyEnvName("MOONSHOT_API_KEY") {
		t.Fatal("expected valid env name")
	}
	if ValidAPIKeyEnvName("sk-secret") {
		t.Fatal("sk- prefix must not be valid env name")
	}
}

func TestAPIKeyEnvLooksLikeSecret(t *testing.T) {
	if !APIKeyEnvLooksLikeSecret("sk-nInCqsCXhIaspnYbCx5HI1KJ1DJiFm9s6zSHxxF0DT5DcUYu") {
		t.Fatal("sk- value should look like secret")
	}
	if APIKeyEnvLooksLikeSecret("MOONSHOT_API_KEY") {
		t.Fatal("proper env name should not look like secret")
	}
}

func TestDefaultAPIKeyEnvForProvider(t *testing.T) {
	cases := map[string]string{
		"kimi-coding-vision": "KIMI_API_KEY",
		"moonshot-vision":    "MOONSHOT_API_KEY",
		"deepseek-flash":     "DEEPSEEK_API_KEY",
	}
	for name, want := range cases {
		if got := DefaultAPIKeyEnvForProvider(name); got != want {
			t.Fatalf("DefaultAPIKeyEnvForProvider(%q) = %q, want %q", name, got, want)
		}
	}
}
