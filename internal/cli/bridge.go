package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"reasonix/internal/bridge"
	"reasonix/internal/config"
	"reasonix/internal/serve"
)

func bridgeCommand(args []string) int {
	fs := flag.NewFlagSet("bridge", flag.ContinueOnError)
	addr := fs.String("addr", "", "reasonix serve address (overrides [bridge].addr)")
	serveOnly := fs.Bool("serve-only", false, "only start reasonix serve, not the bridge sidecar")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !cfg.Bridge.Enabled {
		fmt.Fprintln(os.Stderr, "bridge is disabled; set [bridge].enabled = true in reasonix.toml")
		return 1
	}
	if *addr != "" {
		cfg.Bridge.Addr = *addr
	}
	if cfg.Bridge.Addr == "" {
		cfg.Bridge.Addr = "127.0.0.1:8787"
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	bc := serve.NewBroadcaster()
	ctrl, err := setup(ctx, "", 0, false, bc)
	if err != nil {
		fmt.Fprintln(os.Stderr, "boot:", err)
		return 1
	}
	defer ctrl.Close()

	go func() {
		if err := serve.New(ctrl, bc).RunGraceful(ctx, cfg.Bridge.Addr); err != nil {
			fmt.Fprintln(os.Stderr, "serve:", err)
		}
	}()

	if *serveOnly {
		fmt.Printf("reasonix serve listening on http://%s (bridge sidecar skipped)\n", cfg.Bridge.Addr)
		<-ctx.Done()
		return 0
	}

	cmd, err := bridge.Start(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("bridge started (Feishu/WeChat/QQ) → http://%s\n", cfg.Bridge.Addr)
	<-ctx.Done()
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	return 0
}
