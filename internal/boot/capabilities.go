package boot

import (
	"fmt"
	"io"

	"reasonix/internal/browser"
	"reasonix/internal/capabilities"
	"reasonix/internal/computer"
	"reasonix/internal/config"
	"reasonix/internal/tool"
	"reasonix/internal/vision"
)

// wireCapabilities initializes openhanako-ported services and filters the tool
// registry so disabled capabilities are not exposed to the model.
func wireCapabilities(cfg *config.Config, reg *tool.Registry, stderr io.Writer) {
	var (
		vb  *vision.Bridge
		bm  *browser.Manager
		ch  *computer.Host
		err error
	)

	if cfg.Vision.Enabled {
		vb, err = vision.NewBridge(cfg)
		if err != nil {
			fmt.Fprintf(stderr, "warning: vision bridge disabled: %v\n", err)
		}
	}
	if cfg.Browser.Enabled {
		bm = browser.NewManager(cfg.Browser.Headless, cfg.Browser.Chrome)
	}
	if cfg.Computer.Enabled {
		if computer.Enabled() {
			ch = computer.NewHost(cfg.Computer.AllowWindowsInputInjection)
		} else {
			fmt.Fprintf(stderr, "warning: computer use is not supported on this platform\n")
		}
	}

	capabilities.Init(cfg, vb, bm, ch)
	syncCapabilityTools(reg, cfg)
}

// syncCapabilityTools ensures openhanako-ported tools are present or absent based
// on config. Desktop tabs use Workspace.Tools() which omits these tools, so we
// explicitly add/remove them after the base registry is built.
func syncCapabilityTools(reg *tool.Registry, cfg *config.Config) {
	tools := map[string]bool{
		"understand_image": cfg.Vision.Enabled && capabilities.VisionBridge() != nil,
		"browser":          cfg.Browser.Enabled && capabilities.BrowserManager() != nil,
		"computer":         cfg.Computer.Enabled && capabilities.ComputerHost() != nil,
		"screenshot":       cfg.Computer.Enabled && capabilities.ComputerHost() != nil,
	}
	for name, want := range tools {
		if want {
			if t, ok := tool.LookupBuiltin(name); ok {
				reg.Add(t)
			}
		} else {
			reg.Remove(name)
		}
	}
}
