// Command reasonix is a config- and plugin-driven coding agent CLI.
package main

import (
	"os"

	"deepseek-anka/internal/cli"

	// Blank imports wire compile-time built-ins into their registries.
	_ "deepseek-anka/internal/provider/anthropic"
	_ "deepseek-anka/internal/provider/openai"
	_ "deepseek-anka/internal/tool/builtin"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(cli.Run(os.Args[1:], version))
}
