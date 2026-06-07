import re
import pathlib

cfg = pathlib.Path(r"C:\Users\Administrator\AppData\Roaming\deepseek-anka\config.toml")
cred = pathlib.Path(r"C:\Users\Administrator\AppData\Roaming\deepseek-anka\credentials")
text = cfg.read_text(encoding="utf-8")
pat = re.compile(
    r'(name\s*=\s*"kimi-coding-vision".*?api_key_env\s*=\s*)"(sk-[^"]+)"',
    re.S,
)
m = pat.search(text)
if not m:
    print("No sk- api_key_env found (may already be fixed)")
    raise SystemExit(0)

secret = m.group(2)
text = text[: m.start(2) - 1] + "KIMI_API_KEY" + text[m.end(2) + 1 :]
cfg.write_text(text, encoding="utf-8")

lines = cred.read_text(encoding="utf-8").splitlines() if cred.exists() else []
key = "KIMI_API_KEY"
replaced = False
for i, ln in enumerate(lines):
    if ln.startswith(key + "="):
        lines[i] = key + "=" + secret
        replaced = True
        break
if not replaced:
    lines.append(key + "=" + secret)
cred.parent.mkdir(parents=True, exist_ok=True)
cred.write_text("\n".join(lines) + "\n", encoding="utf-8")
print("Fixed kimi-coding-vision api_key_env -> KIMI_API_KEY")
