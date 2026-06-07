// Package capabilities holds runtime capability services (vision, browser, computer)
// configured at boot and consumed by built-in tools and reference resolution.
package capabilities

import (
	"sync"
	"strings"

	"deepseek-anka/internal/browser"
	"deepseek-anka/internal/computer"
	"deepseek-anka/internal/config"
	"deepseek-anka/internal/vision"
)

var (
	mu sync.RWMutex

	cfg      *config.Config
	visionBr *vision.Bridge
	browserM *browser.Manager
	computerH *computer.Host
	workspaceRoot string
)

// WorkspaceRoot returns the active tab workspace used for attachment resolution.
func WorkspaceRoot() string {
	mu.RLock()
	defer mu.RUnlock()
	return workspaceRoot
}

// Init wires capability services from the loaded config. Safe to call once per boot.
func Init(c *config.Config, visionBridge *vision.Bridge, browserMgr *browser.Manager, computerHost *computer.Host) {
	mu.Lock()
	defer mu.Unlock()
	cfg = c
	visionBr = visionBridge
	browserM = browserMgr
	computerH = computerHost
}

// SetWorkspaceRoot updates the workspace root for attachment/vision path resolution.
func SetWorkspaceRoot(root string) {
	mu.Lock()
	defer mu.Unlock()
	workspaceRoot = strings.TrimSpace(root)
}

// Config returns the active config snapshot.
func Config() *config.Config {
	mu.RLock()
	defer mu.RUnlock()
	return cfg
}

// VisionBridge returns the auxiliary vision bridge, or nil when disabled.
func VisionBridge() *vision.Bridge {
	mu.RLock()
	defer mu.RUnlock()
	return visionBr
}

// BrowserManager returns the shared browser manager.
func BrowserManager() *browser.Manager {
	mu.RLock()
	defer mu.RUnlock()
	return browserM
}

// ComputerHost returns the computer-use host.
func ComputerHost() *computer.Host {
	mu.RLock()
	defer mu.RUnlock()
	return computerH
}

// VisionEnabled reports whether auxiliary vision is active.
func VisionEnabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return cfg != nil && cfg.Vision.Enabled && visionBr != nil
}

// BrowserEnabled reports whether the browser tool is enabled.
func BrowserEnabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return cfg != nil && cfg.Browser.Enabled && browserM != nil
}

// ComputerEnabled reports whether the computer tool is enabled.
func ComputerEnabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return cfg != nil && cfg.Computer.Enabled && computerH != nil
}
