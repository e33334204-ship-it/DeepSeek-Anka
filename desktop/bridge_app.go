package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"deepseek-anka/internal/boot"
	"deepseek-anka/internal/bridge"
	"deepseek-anka/internal/config"
	"deepseek-anka/internal/serve"
)

// BridgePlatformStatusView is one IM platform row for the settings panel.
type BridgePlatformStatusView struct {
	Platform   string `json:"platform"`
	Enabled    bool   `json:"enabled"`
	Configured bool   `json:"configured"`
}

// BridgeStatusView reports bridge sidecar / serve state for the desktop UI.
type BridgeStatusView struct {
	BridgeEnabled  bool                       `json:"bridgeEnabled"`
	SidecarRunning bool                       `json:"sidecarRunning"`
	ServeRunning   bool                       `json:"serveRunning"`
	Addr           string                     `json:"addr"`
	Platforms      []BridgePlatformStatusView `json:"platforms"`
	Error          string                     `json:"error,omitempty"`
}

type bridgeRuntime struct {
	mu      sync.Mutex
	cancel  context.CancelFunc
	cmd     *exec.Cmd
	running bool
	err     string
}

func (a *App) bridgeRT() *bridgeRuntime {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.bridge == nil {
		a.bridge = &bridgeRuntime{}
	}
	return a.bridge
}

// BridgeStatus returns live bridge sidecar state for the settings UI.
func (a *App) BridgeStatus() BridgeStatusView {
	cfg, _, err := a.loadDesktopUserConfigForEdit()
	if err != nil {
		return BridgeStatusView{Error: err.Error()}
	}
	rt := a.bridgeRT()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return bridgeStatusFromConfig(cfg, rt.running, rt.err)
}

func bridgeStatusFromConfig(cfg *config.Config, sidecarRunning bool, sidecarErr string) BridgeStatusView {
	if cfg == nil {
		return BridgeStatusView{}
	}
	addr := strings.TrimSpace(cfg.Bridge.Addr)
	if addr == "" {
		addr = "127.0.0.1:8787"
	}
	out := BridgeStatusView{
		BridgeEnabled:  cfg.Bridge.Enabled,
		SidecarRunning: sidecarRunning,
		ServeRunning:   sidecarRunning,
		Addr:           addr,
		Platforms: []BridgePlatformStatusView{
			{
				Platform:   "feishu",
				Enabled:    cfg.Bridge.Feishu.Enabled,
				Configured: strings.TrimSpace(cfg.Bridge.Feishu.AppID) != "" && strings.TrimSpace(cfg.Bridge.Feishu.AppSecret) != "",
			},
			{
				Platform:   "wechat",
				Enabled:    cfg.Bridge.WeChat.Enabled,
				Configured: strings.TrimSpace(cfg.Bridge.WeChat.BotToken) != "",
			},
			{
				Platform:   "qq",
				Enabled:    cfg.Bridge.QQ.Enabled,
				Configured: strings.TrimSpace(cfg.Bridge.QQ.AppID) != "" && strings.TrimSpace(cfg.Bridge.QQ.AppSecret) != "",
			},
		},
	}
	if sidecarErr != "" {
		out.Error = sidecarErr
	}
	return out
}

func bridgeHasActivePlatform(cfg *config.Config) bool {
	if cfg == nil || !cfg.Bridge.Enabled {
		return false
	}
	if cfg.Bridge.Feishu.Enabled && strings.TrimSpace(cfg.Bridge.Feishu.AppID) != "" && strings.TrimSpace(cfg.Bridge.Feishu.AppSecret) != "" {
		return true
	}
	if cfg.Bridge.WeChat.Enabled && strings.TrimSpace(cfg.Bridge.WeChat.BotToken) != "" {
		return true
	}
	if cfg.Bridge.QQ.Enabled && strings.TrimSpace(cfg.Bridge.QQ.AppID) != "" && strings.TrimSpace(cfg.Bridge.QQ.AppSecret) != "" {
		return true
	}
	return false
}

func (a *App) stopBridgeSidecarLocked(rt *bridgeRuntime) {
	if rt.cancel != nil {
		rt.cancel()
		rt.cancel = nil
	}
	if rt.cmd != nil && rt.cmd.Process != nil {
		_ = rt.cmd.Process.Kill()
		rt.cmd = nil
	}
	rt.running = false
}

// syncBridgeSidecar (re)starts the Node bridge sidecar when enabled in config.
func (a *App) syncBridgeSidecar() {
	cfg, _, err := a.loadDesktopUserConfigForEdit()
	rt := a.bridgeRT()
	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.err = ""
	a.stopBridgeSidecarLocked(rt)

	if err != nil || !bridgeHasActivePlatform(cfg) {
		return
	}
	addr := strings.TrimSpace(cfg.Bridge.Addr)
	if addr == "" {
		addr = "127.0.0.1:8787"
		cfg.Bridge.Addr = addr
	}

	ctx, cancel := context.WithCancel(a.bootContext())
	rt.cancel = cancel

	bc := serve.NewBroadcaster()
	ctrl, err := boot.Build(a.bootContext(), boot.Options{
		Model:         cfg.DefaultModel,
		RequireKey:    false,
		Sink:          bc,
		WorkspaceRoot: a.activeWorkspaceRoot(),
	})
	if err != nil {
		rt.err = fmt.Sprintf("bridge boot: %v", err)
		cancel()
		rt.cancel = nil
		return
	}

	go func() {
		<-ctx.Done()
		ctrl.Close()
	}()

	go func() {
		if err := serve.New(ctrl, bc).RunGraceful(ctx, addr); err != nil && ctx.Err() == nil {
			rt.mu.Lock()
			rt.err = fmt.Sprintf("bridge serve: %v", err)
			rt.running = false
			rt.mu.Unlock()
		}
	}()

	cmd, err := bridge.Start(cfg)
	if err != nil {
		rt.err = err.Error()
		cancel()
		rt.cancel = nil
		return
	}
	rt.cmd = cmd
	rt.running = true

	go func() {
		waitErr := cmd.Wait()
		rt.mu.Lock()
		defer rt.mu.Unlock()
		if ctx.Err() != nil {
			return
		}
		rt.running = false
		if waitErr != nil {
			rt.err = waitErr.Error()
		}
	}()
}

// RestartBridge stops and restarts the bridge sidecar from current config.
func (a *App) RestartBridge() error {
	a.syncBridgeSidecar()
	rt := a.bridgeRT()
	rt.mu.Lock()
	errMsg := rt.err
	rt.mu.Unlock()
	if errMsg != "" {
		return fmt.Errorf("%s", errMsg)
	}
	return nil
}
