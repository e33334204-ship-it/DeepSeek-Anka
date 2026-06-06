/**
 * Bridge adapters ported from openhanako (lib/bridge/*).
 * Routes inbound platform messages to Reasonix serve and sends replies back.
 */

import * as lark from "@larksuiteoapi/node-sdk";
import WebSocket from "ws";
import { ReasonixClient } from "./reasonix-client.js";

export function createBridgeManager(config, reasonixBaseUrl) {
  const reasonix = new ReasonixClient(reasonixBaseUrl);
  const adapters = [];

  if (config.feishu?.enabled) {
    adapters.push(createFeishuAdapter(config.feishu, reasonix, config.owner));
  }
  if (config.qq?.enabled) {
    adapters.push(createQQAdapter(config.qq, reasonix, config.owner));
  }
  if (config.wechat?.enabled) {
    adapters.push(createWeChatAdapter(config.wechat, reasonix, config.owner));
  }

  return {
    async start() {
      for (const a of adapters) await a.start();
      console.log(`[bridge] started ${adapters.length} platform adapter(s) → ${reasonixBaseUrl}`);
    },
    async stop() {
      for (const a of adapters) await a.stop?.();
    },
  };
}

function createFeishuAdapter(cfg, reasonix, owner) {
  const client = new lark.Client({
    appId: cfg.app_id,
    appSecret: cfg.app_secret,
    appType: lark.AppType.SelfBuild,
    domain: lark.Domain.Feishu,
  });
  const wsClient = new lark.WSClient({
    appId: cfg.app_id,
    appSecret: cfg.app_secret,
    domain: lark.Domain.Feishu,
  });

  const handler = async (data) => {
    try {
      const event = data?.event;
      if (!event?.message) return;
      const msg = event.message;
      if (msg.message_type !== "text") return;
      const userId = event.sender?.sender_id?.open_id || "";
      if (owner && userId && userId !== owner) return;
      let text = "";
      try {
        text = JSON.parse(msg.content)?.text || "";
      } catch {
        text = msg.content || "";
      }
      text = String(text).trim();
      if (!text) return;
      const reply = await reasonix.runTurn(`[feishu:${userId}] ${text}`);
      await client.im.message.reply({
        path: { message_id: msg.message_id },
        data: { content: JSON.stringify({ text: reply.slice(0, 4000) }), msg_type: "text" },
      });
    } catch (err) {
      console.error("[feishu] handler error:", err);
    }
  };

  return {
    name: "feishu",
    async start() {
      wsClient.start({ eventDispatcher: new lark.EventDispatcher({}).register({ "im.message.receive_v1": handler }) });
      console.log("[feishu] WS client started");
    },
    async stop() {
      wsClient.stop?.();
    },
  };
}

function createQQAdapter(cfg, reasonix, owner) {
  let ws;
  let token = "";
  let heartbeatTimer;

  async function refreshToken() {
    const res = await fetch("https://bots.qq.com/app/getAppAccessToken", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ appId: cfg.app_id, clientSecret: cfg.app_secret }),
    });
    const data = await res.json();
    token = data.access_token;
    return token;
  }

  async function handleMessage(msg) {
    if (msg.t !== "C2C_MESSAGE_CREATE" && msg.t !== "AT_MESSAGE_CREATE") return;
    const d = msg.d || {};
    const userId = d.author?.id || d.author?.member_openid || "";
    if (owner && userId && userId !== owner) return;
    const text = String(d.content || "").replace(/<@![^>]+>/g, "").trim();
    if (!text) return;
    const reply = await reasonix.runTurn(`[qq:${userId}] ${text}`);
    const guildId = cfg.dm_guild_map?.[userId] || d.guild_id;
    await fetch(`https://api.sgroup.qq.com/v2/users/${userId}/messages`, {
      method: "POST",
      headers: { Authorization: `QQBot ${token}`, "content-type": "application/json" },
      body: JSON.stringify({ content: reply.slice(0, 2000), msg_type: 0, guild_id: guildId }),
    });
  }

  return {
    name: "qq",
    async start() {
      await refreshToken();
      const res = await fetch("https://api.sgroup.qq.com/gateway", {
        headers: { Authorization: `QQBot ${token}` },
      });
      const gw = await res.json();
      ws = new WebSocket(gw.url);
      ws.on("message", async (raw) => {
        const msg = JSON.parse(String(raw));
        if (msg.op === 10) {
          heartbeatTimer = setInterval(() => ws.send(JSON.stringify({ op: 1, d: null })), (msg.d?.heartbeat_interval || 45000));
          ws.send(JSON.stringify({ op: 2, d: { token: `QQBot ${token}`, intents: 1 << 25 | 1 << 12 } }));
        } else if (msg.op === 0) {
          await handleMessage(msg);
        }
      });
      console.log("[qq] gateway connected");
    },
    async stop() {
      clearInterval(heartbeatTimer);
      ws?.close();
    },
  };
}

function createWeChatAdapter(cfg, reasonix, owner) {
  // WeChat iLink long-poll (simplified port from openhanako wechat-adapter.js)
  let running = false;
  let pollTimer;

  async function poll() {
    if (!running) return;
    try {
      const res = await fetch(`https://ilinkai.weixin.qq.com/cgi-bin/message?access_token=${cfg.bot_token}&format=json`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ longpolling: 30 }),
      });
      const data = await res.json();
      const msgs = data?.MessageList || data?.message_list || [];
      for (const m of msgs) {
        const userId = m.FromUserName || m.from_user_name || "";
        if (owner && userId && userId !== owner) continue;
        const text = String(m.Content || m.content || "").trim();
        if (!text) continue;
        const reply = await reasonix.runTurn(`[wechat:${userId}] ${text}`);
        await fetch(`https://ilinkai.weixin.qq.com/cgi-bin/message/send?access_token=${cfg.bot_token}`, {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ touser: userId, msgtype: "text", text: { content: reply.slice(0, 2000) } }),
        });
      }
    } catch (err) {
      console.error("[wechat] poll error:", err);
    }
    pollTimer = setTimeout(poll, 1000);
  }

  return {
    name: "wechat",
    async start() {
      if (!cfg.bot_token) throw new Error("wechat.bot_token is required");
      running = true;
      poll();
      console.log("[wechat] long-poll started");
    },
    async stop() {
      running = false;
      clearTimeout(pollTimer);
    },
  };
}
