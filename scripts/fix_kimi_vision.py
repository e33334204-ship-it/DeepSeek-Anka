"""Switch vision config from Moonshot platform to Kimi Coding (matches openhanako)."""
from __future__ import annotations

import pathlib
import re

CFG = pathlib.Path.home() / "AppData/Roaming/deepseek-anka/config.toml"

KIMI_PROVIDER = '''
[[providers]]
name        = "kimi-coding-vision"
kind        = "openai"
base_url    = "https://api.kimi.com/coding/v1"
models      = ["kimi-k2.6", "kimi-for-coding", "kimi-k2.5", "kimi-k2"]
default     = "kimi-k2.6"
api_key_env = "KIMI_API_KEY"
context_window = 128000
'''

def main() -> None:
    text = CFG.read_text(encoding="utf-8")
    if "kimi-coding-vision" not in text:
        text = text.replace("\n[tools]", KIMI_PROVIDER + "\n[tools]", 1)
    text = re.sub(
        r'(\[vision\]\s*\nenabled\s*=\s*true\s*\nmodel\s*=\s*)"moonshot-vision/kimi-k2\.6"',
        r'\1"kimi-coding-vision/kimi-k2.6"',
        text,
    )
    CFG.write_text(text, encoding="utf-8")
    print("vision -> kimi-coding-vision/kimi-k2.6")

if __name__ == "__main__":
    main()
