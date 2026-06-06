package config

import (
	"fmt"
	"strings"
)
// VisionConfig controls auxiliary vision analysis for text-only chat models.
// When enabled, image attachments are analyzed by a separate vision-capable
// provider and the notes are injected into the prompt before the main model runs.
//
// Model is required when Enabled is true. It must reference a vision-capable
// [[providers]] entry (e.g. "gpt-4o" or "dashscope/qwen-vl-max") — not the
// default chat model. This mirrors openhanako's vision_model setting.
type VisionConfig struct {
	Enabled bool   `toml:"enabled"`
	Model   string `toml:"model"` // provider name or provider/model ref; required when enabled
}

// ResolveVisionModel returns the provider entry for the configured vision model.
// When vision is disabled, ok is false with no error. When enabled but Model is
// empty or unknown, returns an error — vision must never silently fall back to
// default_model.
func (c *Config) ResolveVisionModel() (*ProviderEntry, error) {
	if c == nil || !c.Vision.Enabled {
		return nil, nil
	}
	ref := strings.TrimSpace(c.Vision.Model)
	if ref == "" {
		return nil, fmt.Errorf("[vision].model is required when [vision].enabled = true — configure a vision-capable provider (e.g. gpt-4o, qwen-vl-max)")
	}
	entry, ok := c.ResolveModel(ref)
	if !ok {
		return nil, fmt.Errorf("[vision].model %q not found in [[providers]]", ref)
	}
	if entry.APIKeyEnv != "" && entry.APIKey() == "" {
		return nil, fmt.Errorf("[vision] API key %s is not set for model %q", entry.APIKeyEnv, ref)
	}
	return entry, nil
}

// ComputerConfig controls desktop automation via the computer built-in tool.
type ComputerConfig struct {
	Enabled                    bool `toml:"enabled"`
	AllowWindowsInputInjection bool `toml:"allow_windows_input_injection"`
}

// BrowserConfig controls the headless browser built-in tool.
type BrowserConfig struct {
	Enabled  bool   `toml:"enabled"`
	Headless bool   `toml:"headless"` // run Chromium headless; false shows a visible window
	Chrome   string `toml:"chrome"`   // optional path to Chrome/Chromium executable
}

// BridgeConfig controls external IM platform adapters (Feishu, WeChat, QQ).
// The bridge sidecar connects inbound messages to reasonix serve and streams
// replies back to the originating platform.
type BridgeConfig struct {
	Enabled bool              `toml:"enabled"`
	Addr    string            `toml:"addr"` // reasonix serve address the bridge calls; default 127.0.0.1:8787
	Feishu  BridgeFeishuConfig `toml:"feishu"`
	WeChat  BridgeWeChatConfig `toml:"wechat"`
	QQ      BridgeQQConfig     `toml:"qq"`
}

// BridgeFeishuConfig holds Feishu/Lark bot credentials.
type BridgeFeishuConfig struct {
	Enabled   bool   `toml:"enabled"`
	AppID     string `toml:"app_id"`
	AppSecret string `toml:"app_secret"`
	Owner     string `toml:"owner"` // owner open_id; only owner can remote-control by default
}

// BridgeWeChatConfig holds WeChat iLink bot credentials.
type BridgeWeChatConfig struct {
	Enabled  bool   `toml:"enabled"`
	BotToken string `toml:"bot_token"`
	Owner    string `toml:"owner"`
}

// BridgeQQConfig holds QQ Bot v2 credentials.
type BridgeQQConfig struct {
	Enabled   bool              `toml:"enabled"`
	AppID     string            `toml:"app_id"`
	AppSecret string            `toml:"app_secret"`
	Owner     string            `toml:"owner"`
	DMGuildMap map[string]string `toml:"dm_guild_map"` // userId -> guildId for C2C routing
}
