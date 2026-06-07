import { isImageCapableRef } from "./visionModels";
import type { ProviderView, SettingsView } from "./types";

export interface ComposerImageAttachment {
  path: string;
  previewUrl?: string;
}

export interface ComposerWorkspaceRef {
  path: string;
  isDir?: boolean;
}

export type ModelImageInputMode = "native-image" | "text-only" | "unknown";

export type ChatImageSendPreflightResult =
  | {
      ok: true;
      reason: "no-images" | "native-image" | "unknown-model-capability" | "auxiliary-vision";
      imageInputMode: ModelImageInputMode;
    }
  | {
      ok: false;
      reason: "text-model-image-without-auxiliary";
      imageInputMode: "text-only";
    };

const IMAGE_EXT = /\.(png|jpe?g|gif|webp)$/i;

function isImagePath(path: string): boolean {
  const clean = path.split(/[?#]/)[0] ?? path;
  return IMAGE_EXT.test(clean) || clean.includes("/attachments/");
}

export function hasComposerImageAttachments(
  attachments: readonly ComposerImageAttachment[],
  workspaceRefs: readonly ComposerWorkspaceRef[],
): boolean {
  if (attachments.some((a) => a.previewUrl || isImagePath(a.path))) return true;
  return workspaceRefs.some((r) => !r.isDir && isImagePath(r.path));
}

function findProvider(ref: string, providers: readonly ProviderView[]): ProviderView | undefined {
  const trimmed = ref.trim();
  if (!trimmed) return undefined;
  const slash = trimmed.indexOf("/");
  if (slash > 0) {
    const name = trimmed.slice(0, slash);
    return providers.find((p) => p.name === name);
  }
  return providers.find((p) => p.models.includes(trimmed) || p.default === trimmed);
}

function baseHost(raw: string): string {
  const trimmed = raw.trim().toLowerCase();
  if (!trimmed) return "";
  try {
    const url = trimmed.includes("://") ? new URL(trimmed) : new URL(`https://${trimmed}`);
    return url.hostname;
  } catch {
    return trimmed.split("/")[0] ?? "";
  }
}

function isOfficialDeepSeekProvider(provider: ProviderView): boolean {
  if (provider.name.toLowerCase() === "deepseek") return true;
  return baseHost(provider.baseUrl) === "api.deepseek.com";
}

function isMoonshotOrKimiProvider(provider: ProviderView): boolean {
  const host = baseHost(provider.baseUrl);
  if (host === "api.moonshot.cn" || host === "api.moonshot.ai") return true;
  return host.includes("kimi.com");
}

function chatModelRef(settings: SettingsView | null | undefined): string {
  return settings?.defaultModel?.trim() ?? "";
}

export function getChatModelImageInputMode(settings: SettingsView | null | undefined): ModelImageInputMode {
  const ref = chatModelRef(settings);
  if (!ref) return "unknown";
  const provider = findProvider(ref, settings?.providers ?? []);
  if (!provider) return "unknown";
  if (isOfficialDeepSeekProvider(provider) || isMoonshotOrKimiProvider(provider)) {
    return "text-only";
  }
  return isImageCapableRef(ref, provider.baseUrl) ? "native-image" : "text-only";
}

function canUseVisionAuxiliary(settings: SettingsView | null | undefined): boolean {
  const vision = settings?.vision;
  if (!vision?.enabled || !vision.model?.trim()) return false;
  const provider = findProvider(vision.model, settings?.providers ?? []);
  if (provider && !provider.keySet) return false;
  return true;
}

export function evaluateChatImageSendPreflight({
  attachments,
  workspaceRefs,
  settings,
}: {
  attachments: readonly ComposerImageAttachment[];
  workspaceRefs: readonly ComposerWorkspaceRef[];
  settings: SettingsView | null | undefined;
}): ChatImageSendPreflightResult {
  const imageInputMode = getChatModelImageInputMode(settings);
  if (!hasComposerImageAttachments(attachments, workspaceRefs)) {
    return { ok: true, reason: "no-images", imageInputMode };
  }
  if (imageInputMode === "native-image") {
    return { ok: true, reason: "native-image", imageInputMode };
  }
  if (imageInputMode === "unknown") {
    return { ok: true, reason: "unknown-model-capability", imageInputMode };
  }
  if (canUseVisionAuxiliary(settings)) {
    return { ok: true, reason: "auxiliary-vision", imageInputMode };
  }
  return {
    ok: false,
    reason: "text-model-image-without-auxiliary",
    imageInputMode: "text-only",
  };
}
