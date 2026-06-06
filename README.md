<p align="center">
  <img src="docs/logo.svg" alt="Reasonix" width="640"/>
</p>

<p align="center">
  <strong>English</strong>
  &nbsp;·&nbsp;
  <a href="./README.zh-CN.md">简体中文</a>
  &nbsp;·&nbsp;
  <a href="./docs/SPEC.md">Spec</a>
</p>

> [!IMPORTANT]
> **Reasonix 1.0 已用 Go 完全重写** — 本分支（`main-v2`）是新的默认开发分支。
> 旧版 0.x TypeScript 版本已归档到 [`v1`](https://github.com/esengine/DeepSeek-Reasonix/tree/v1) 分支（仅维护）。

<h3 align="center">A DeepSeek-native AI coding agent for your terminal.</h3>
<p align="center">A config- and plugin-driven harness — a single static Go binary, tuned around DeepSeek's prefix cache so token costs stay low across long sessions.</p>

<br/>

## 增强版新增功能

本项目基于 Reasonix 内核，集成自 openhanako 的以下增强模块：

| 模块 | 功能 |
|------|------|
| 🖼️ **视觉 Pipeline** | 纯文本模型（如 DeepSeek）自动调用辅助视觉模型分析图片，注入 `<vision-context>` 笔记 |
| 🖥️ **Windows UIA** | 完整 UIA 元素树遍历、后台 InvokePattern 点击、ValuePattern 文本输入、滚动 |
| 🌐 **浏览器增强** | URL 冷保存持久化、会话健康监控、一次性 Web 搜索 |
| 💬 **IM 桥接** | 飞书/微信/QQ 消息对接（Node.js sidecar） |

### 架构

```
三个前端 → control.Controller（传输无关）→ 同一套 internal/* 内核
  ├── CLI TUI (bubbletea)
  ├── HTTP/SSE serve
  └── Desktop (Wails + React)
```

所有增强模块自动被三个前端继承，无需额外适配。

## Features

- **Config-driven.** Providers, the agent, enabled tools, and plugins are all
  declared in `reasonix.toml`. No hardcoded models.
- **Multi-model & composable.** DeepSeek and any OpenAI-compatible endpoint.
  Optionally run two models together (executor + planner) in separate,
  cache-stable sessions.
- **Plugin-driven.** External tools run as subprocesses over stdio JSON-RPC
  (MCP-compatible). Built-in tools self-register at compile time.
- **Zero-friction distribution.** `CGO_ENABLED=0` single binary; cross-compile
  to six targets with one command.

## Install

### Prebuilt binary (Windows)

从 [Releases](https://github.com/e33334204-ship-it/DeepSeek-Reasonix/releases) 下载最新 `.exe`。

### Build from source

```sh
go build -o bin/reasonix.exe ./cmd/reasonix      # CLI
cd desktop && wails build                          # Desktop (需要 Wails)
```

## Quick start

```sh
reasonix setup                      # config wizard → ./reasonix.toml
export DEEPSEEK_API_KEY=sk-...  # or put it in .env
reasonix chat                       # then run /init to generate AGENTS.md
reasonix run "implement the TODOs in main.go"
reasonix run --model mimo-pro "add unit tests for this function"
```

## Configuration

Resolution order: flag > `./reasonix.toml` > `~/.config/reasonix/config.toml` >
built-in defaults. Secrets come from the environment via `api_key_env` and are
never stored in config files.

```toml
default_model = "deepseek-flash"

[vision]
# enabled = true
# model = "gpt-4o"
# provider = "openai"

[computer]
# enabled = true

[browser]
# enabled = true

[bridge]
# enabled = true
```

See [docs/SPEC.md](./docs/SPEC.md) for the full schema.

## 赞助支持

如果 Reasonix 对你有所帮助，欢迎打赏支持作者：

<p align="center">
  <img src="docs/sponsor-qr.jpg" alt="打赏码" width="240"/>
</p>

## License

MIT — see [LICENSE](./LICENSE).
