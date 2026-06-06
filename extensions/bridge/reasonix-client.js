/**
 * Reasonix HTTP/SSE client for bridge sidecar.
 * Submits user messages to reasonix serve and collects assistant replies.
 */

export class ReasonixClient {
  /**
   * @param {string} baseUrl e.g. http://127.0.0.1:8787
   */
  constructor(baseUrl) {
    this.baseUrl = baseUrl.replace(/\/$/, "");
  }

  async submit(input) {
    const res = await fetch(`${this.baseUrl}/submit`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ input }),
    });
    if (!res.ok && res.status !== 204) {
      const text = await res.text().catch(() => "");
      throw new Error(`submit failed ${res.status}: ${text}`);
    }
  }

  /**
   * Run one turn: submit input, wait for turn_done on SSE, return assistant text.
   */
  async runTurn(input, timeoutMs = 300000) {
    const events = [];
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), timeoutMs);

    const streamPromise = (async () => {
      const res = await fetch(`${this.baseUrl}/events`, { signal: ctrl.signal });
      if (!res.ok || !res.body) throw new Error(`events stream failed ${res.status}`);
      const reader = res.body.getReader();
      const dec = new TextDecoder();
      let buf = "";
      while (true) {
        const { value, done } = await reader.read();
        if (done) break;
        buf += dec.decode(value, { stream: true });
        let idx;
        while ((idx = buf.indexOf("\n\n")) >= 0) {
          const chunk = buf.slice(0, idx);
          buf = buf.slice(idx + 2);
          for (const line of chunk.split("\n")) {
            if (!line.startsWith("data: ")) continue;
            const raw = line.slice(6).trim();
            if (!raw || raw.startsWith(":")) continue;
            try {
              events.push(JSON.parse(raw));
            } catch {}
          }
        }
      }
    })();

    await this.submit(input);
    let reply = "";
    const started = Date.now();
    while (Date.now() - started < timeoutMs) {
      for (const ev of events.splice(0)) {
        if (ev.kind === "assistant" && ev.text) reply += ev.text;
        if (ev.kind === "turn_done") {
          clearTimeout(timer);
          ctrl.abort();
          await streamPromise.catch(() => {});
          return reply.trim() || "(empty reply)";
        }
      }
      await sleep(200);
    }
    clearTimeout(timer);
    ctrl.abort();
    await streamPromise.catch(() => {});
    throw new Error("reasonix turn timed out");
  }
}

function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms));
}
