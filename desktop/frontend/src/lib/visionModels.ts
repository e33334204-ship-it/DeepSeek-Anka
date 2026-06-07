// Image-capable model filtering for the vision model picker (openhanako OtherModelsSection).

const IMAGE_MODEL_MARKERS = [
  "gpt-4o",
  "gpt-4-turbo",
  "gpt-4.1",
  "gpt-4.5",
  "claude-3",
  "claude-sonnet-4",
  "claude-opus-4",
  "gemini",
  "qwen-vl",
  "qwen2-vl",
  "qwen3-vl",
  "pixtral",
  "llava",
  "glm-4v",
  "yi-vision",
  "gpt-4-vision",
  "vision",
  "-vl",
  "_vl",
  "kimi-k2",
  "kimi-latest",
  "kimi-for-coding",
  "doubao-vision",
  "doubao-seed",
  "grok",
  "llama-4",
  "internvl",
  "step-1v",
  "hunyuan-vision",
  "glm-5v",
  "mimo-v2-omni",
  "minimax-m3",
] as const;

const DEEPSEEK_HOST_MARKERS = ["deepseek", "api.deepseek.com"] as const;

/** Returns true when a provider/model ref likely supports direct image input. */
export function isImageCapableRef(ref: string, providerBaseUrl = ""): boolean {
  const trimmed = ref.trim();
  if (!trimmed) return false;
  const slash = trimmed.indexOf("/");
  const provider = slash > 0 ? trimmed.slice(0, slash).toLowerCase() : "";
  const model = (slash > 0 ? trimmed.slice(slash + 1) : trimmed).toLowerCase();
  const base = providerBaseUrl.toLowerCase();

  if (DEEPSEEK_HOST_MARKERS.some((m) => provider.includes(m) || base.includes(m))) {
    return false;
  }
  return IMAGE_MODEL_MARKERS.some((m) => model.includes(m) || provider.includes(m));
}

export function visionModelRefs(
  providers: { name: string; models: string[]; baseUrl?: string }[],
): string[] {
  const out: string[] = [];
  for (const p of providers) {
    const base = p.baseUrl || "";
    for (const m of p.models) {
      const ref = `${p.name}/${m}`;
      if (isImageCapableRef(ref, base)) out.push(ref);
    }
  }
  return out;
}

export function parseModelRef(raw: string): { id: string; provider: string } | null {
  const s = raw.trim();
  if (!s) return null;
  const slash = s.indexOf("/");
  if (slash > 0 && slash < s.length - 1) {
    return { provider: s.slice(0, slash), id: s.slice(slash + 1) };
  }
  return { id: s, provider: "" };
}

/** Human-friendly label for the vision model picker (openhanako-style). */
export function formatVisionModelLabel(ref: string): string {
  const parsed = parseModelRef(ref);
  if (!parsed) return ref;
  const id = parsed.id;
  if (/^kimi-k2/i.test(id)) {
    const ver = id.replace(/^kimi-k2\.?/i, "").replace(/^kimi-/i, "");
    return ver ? `Kimi K2.${ver}` : "Kimi K2";
  }
  if (id === "kimi-latest") return "Kimi Latest";
  if (id === "kimi-for-coding") return "Kimi for Coding";
  if (/^gpt-4o/i.test(id)) return id.toUpperCase().replace("GPT-4O", "GPT-4o");
  if (/^qwen-vl/i.test(id)) return id.replace(/-/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
  if (/^moonshot-v1.*vision/i.test(id)) return "Moonshot Vision";
  return id.replace(/-/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}
