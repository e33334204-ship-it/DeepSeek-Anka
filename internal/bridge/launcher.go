package bridge

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"deepseek-anka/internal/config"
)

// ConfigJSON is the bridge sidecar configuration written for the Node process.
type ConfigJSON struct {
	Owner  string                 `json:"owner"`
	Feishu configBridgeFeishu     `json:"feishu"`
	WeChat configBridgeWeChat     `json:"wechat"`
	QQ     configBridgeQQ         `json:"qq"`
}

type configBridgeFeishu struct {
	Enabled   bool   `json:"enabled"`
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

type configBridgeWeChat struct {
	Enabled  bool   `json:"enabled"`
	BotToken string `json:"bot_token"`
}

type configBridgeQQ struct {
	Enabled    bool              `json:"enabled"`
	AppID      string            `json:"app_id"`
	AppSecret  string            `json:"app_secret"`
	DMGuildMap map[string]string `json:"dm_guild_map"`
}

// ExtensionsDir returns the extensions/bridge directory (repo root or install layout).
func ExtensionsDir() (string, error) {
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for i := 0; i < 6; i++ {
			candidates = append(candidates, filepath.Join(dir, "extensions", "bridge"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	if wd, err := os.Getwd(); err == nil {
		dir := wd
		for i := 0; i < 6; i++ {
			candidates = append(candidates, filepath.Join(dir, "extensions", "bridge"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	seen := map[string]bool{}
	for _, c := range candidates {
		if seen[c] {
			continue
		}
		seen[c] = true
		if st, err := os.Stat(filepath.Join(c, "index.js")); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("extensions/bridge not found; run from DeepSeek-Anka repo root or install extensions/bridge beside the binary")
}

// WriteConfigFile serializes bridge config for the Node sidecar.
func WriteConfigFile(dir string, cfg *config.Config) (string, error) {
	out := ConfigJSON{
		Owner: firstOwner(cfg),
		Feishu: configBridgeFeishu{
			Enabled:   cfg.Bridge.Feishu.Enabled,
			AppID:     cfg.Bridge.Feishu.AppID,
			AppSecret: cfg.Bridge.Feishu.AppSecret,
		},
		WeChat: configBridgeWeChat{
			Enabled:  cfg.Bridge.WeChat.Enabled,
			BotToken: cfg.Bridge.WeChat.BotToken,
		},
		QQ: configBridgeQQ{
			Enabled:    cfg.Bridge.QQ.Enabled,
			AppID:      cfg.Bridge.QQ.AppID,
			AppSecret:  cfg.Bridge.QQ.AppSecret,
			DMGuildMap: cfg.Bridge.QQ.DMGuildMap,
		},
	}
	path := filepath.Join(dir, "bridge.runtime.json")
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func firstOwner(cfg *config.Config) string {
	if cfg.Bridge.Feishu.Owner != "" {
		return cfg.Bridge.Feishu.Owner
	}
	if cfg.Bridge.WeChat.Owner != "" {
		return cfg.Bridge.WeChat.Owner
	}
	return cfg.Bridge.QQ.Owner
}

// Start launches the Node bridge sidecar. Caller should run reasonix serve first.
func Start(cfg *config.Config) (*exec.Cmd, error) {
	dir, err := ExtensionsDir()
	if err != nil {
		return nil, err
	}
	cfgPath, err := WriteConfigFile(dir, cfg)
	if err != nil {
		return nil, err
	}
	node := "node"
	if runtime.GOOS == "windows" {
		node = "node.exe"
	}
	cmd := exec.Command(node, "index.js")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"REASONIX_BRIDGE_CONFIG="+cfgPath,
		"REASONIX_SERVE_URL=http://"+cfg.Bridge.Addr,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start bridge sidecar: %w", err)
	}
	return cmd, nil
}
