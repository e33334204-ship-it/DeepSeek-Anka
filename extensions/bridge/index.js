#!/usr/bin/env node
/**
 * Reasonix bridge sidecar — Feishu / WeChat / QQ integration.
 * Ported from openhanako lib/bridge (BridgeManager + platform adapters).
 *
 * Usage:
 *   REASONIX_BRIDGE_CONFIG=/path/to/bridge.json node index.js
 *   REASONIX_SERVE_URL=http://127.0.0.1:8787 node index.js
 */

import fs from "fs";
import path from "path";
import { createBridgeManager } from "./adapters.js";

const serveUrl = process.env.REASONIX_SERVE_URL || "http://127.0.0.1:8787";
const configPath = process.env.REASONIX_BRIDGE_CONFIG || findBridgeConfig();

function findBridgeConfig() {
  const candidates = ["bridge.json", "reasonix-bridge.json", path.join(process.cwd(), "bridge.json")];
  for (const c of candidates) {
    if (fs.existsSync(c)) return c;
  }
  return "";
}

function loadConfig() {
  if (configPath && fs.existsSync(configPath)) {
    return JSON.parse(fs.readFileSync(configPath, "utf8"));
  }
  return {
    owner: process.env.REASONIX_BRIDGE_OWNER || "",
    feishu: {
      enabled: process.env.FEISHU_ENABLED === "1",
      app_id: process.env.FEISHU_APP_ID || "",
      app_secret: process.env.FEISHU_APP_SECRET || "",
    },
    wechat: {
      enabled: process.env.WECHAT_ENABLED === "1",
      bot_token: process.env.WECHAT_BOT_TOKEN || "",
    },
    qq: {
      enabled: process.env.QQ_ENABLED === "1",
      app_id: process.env.QQ_APP_ID || "",
      app_secret: process.env.QQ_APP_SECRET || "",
      dm_guild_map: {},
    },
  };
}

const config = loadConfig();
const manager = createBridgeManager(config, serveUrl);

manager.start().catch((err) => {
  console.error("[bridge] fatal:", err);
  process.exit(1);
});

process.on("SIGINT", async () => {
  await manager.stop();
  process.exit(0);
});
