# Reasonix Bridge Sidecar

Feishu / WeChat / QQ integration for DeepSeek-Reasonix (ported from openhanako).

## Prerequisites

1. Configure `[bridge]` in `reasonix.toml` (see `reasonix.example.toml`)
2. Configure platform credentials under `[bridge.feishu]`, `[bridge.wechat]`, `[bridge.qq]`
3. Install Node.js dependencies:

```bash
cd extensions/bridge
npm install
```

## Run

Start bridge (includes reasonix serve + platform adapters):

```bash
reasonix bridge
```

Or serve only:

```bash
reasonix bridge --serve-only
```

## Vision note

Image messages from IM platforms are forwarded as text to Reasonix. For image understanding,
configure auxiliary vision separately in `reasonix.toml`:

```toml
[vision]
enabled = true
model   = "gpt-4o"   # must be a vision-capable provider, NOT your chat model
```
