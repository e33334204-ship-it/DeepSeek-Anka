import type { ProviderView } from "./types";
import type { DictKey } from "../locales/en";

export interface VisionProviderTemplate {
  id: string;
  labelKey: DictKey;
  hintKey: DictKey;
  provider: Omit<ProviderView, "keySet">;
  /** Default vision model id within the provider. */
  visionModel: string;
}

/** One-click vision provider presets (openhanako-style quick setup). */
export const VISION_PROVIDER_TEMPLATES: VisionProviderTemplate[] = [
  {
    id: "openai",
    labelKey: "settings.vision.templateOpenAI",
    hintKey: "settings.vision.templateOpenAIHint",
    provider: {
      name: "openai-vision",
      kind: "openai",
      baseUrl: "https://api.openai.com/v1",
      models: ["gpt-4o", "gpt-4o-mini"],
      default: "gpt-4o",
      apiKeyEnv: "OPENAI_API_KEY",
      balanceUrl: "",
      contextWindow: 128_000,
    },
    visionModel: "gpt-4o",
  },
  {
    id: "dashscope",
    labelKey: "settings.vision.templateDashScope",
    hintKey: "settings.vision.templateDashScopeHint",
    provider: {
      name: "dashscope-vision",
      kind: "openai",
      baseUrl: "https://dashscope.aliyuncs.com/compatible-mode/v1",
      models: ["qwen-vl-max", "qwen-vl-plus"],
      default: "qwen-vl-max",
      apiKeyEnv: "DASHSCOPE_API_KEY",
      balanceUrl: "",
      contextWindow: 128_000,
    },
    visionModel: "qwen-vl-max",
  },
  {
    id: "siliconflow",
    labelKey: "settings.vision.templateSiliconFlow",
    hintKey: "settings.vision.templateSiliconFlowHint",
    provider: {
      name: "siliconflow-vision",
      kind: "openai",
      baseUrl: "https://api.siliconflow.cn/v1",
      models: ["Qwen/Qwen2-VL-72B-Instruct", "OpenGVLab/InternVL2-8B"],
      default: "Qwen/Qwen2-VL-72B-Instruct",
      apiKeyEnv: "SILICONFLOW_API_KEY",
      balanceUrl: "",
      contextWindow: 32_000,
    },
    visionModel: "Qwen/Qwen2-VL-72B-Instruct",
  },
];
