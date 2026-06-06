package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"reasonix/internal/capabilities"
	"reasonix/internal/control"
	"reasonix/internal/tool"
	"reasonix/internal/vision"
)

func init() { tool.RegisterBuiltin(browserTool{}) }

type browserTool struct {
	sessionID string
}

func (browserTool) Name() string { return "browser" }

func (browserTool) Description() string {
	return "Control a built-in headless Chromium browser for navigation, page inspection, interaction, screenshots, and web search. Actions: start, stop, health, list_sessions, navigate, snapshot, screenshot, click, type, scroll, evaluate, wait, search."
}

func (browserTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "action":{"type":"string","enum":["start","stop","health","list_sessions","navigate","snapshot","screenshot","click","type","scroll","evaluate","wait","search"],"description":"Browser action to perform"},
  "url":{"type":"string","description":"URL for navigate or search"},
  "ref":{"type":"string","description":"Element ref from snapshot for click/type"},
  "text":{"type":"string","description":"Text to type"},
  "expression":{"type":"string","description":"JavaScript for evaluate"},
  "delta_y":{"type":"number","description":"Scroll amount in pixels"},
  "milliseconds":{"type":"integer","description":"Wait duration for wait action"},
  "session_id":{"type":"string","description":"Override session id (default: auto)"},
  "max_chars":{"type":"integer","description":"Max characters for search content extraction"}
},
"required":["action"]
}`)
}

func (browserTool) ReadOnly() bool { return false }

func (t browserTool) session() string {
	if t.sessionID != "" {
		return t.sessionID
	}
	return "default"
}

func (t browserTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	mgr := capabilities.BrowserManager()
	if mgr == nil {
		return "", fmt.Errorf("browser is disabled; enable [browser] in reasonix.toml")
	}
	var p struct {
		Action       string  `json:"action"`
		URL          string  `json:"url"`
		Ref          string  `json:"ref"`
		Text         string  `json:"text"`
		Expression   string  `json:"expression"`
		DeltaY       float64 `json:"delta_y"`
		Milliseconds int     `json:"milliseconds"`
		SessionID    string  `json:"session_id"`
		MaxChars     int     `json:"max_chars"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	sid := p.SessionID
	if sid == "" {
		sid = t.session()
	}
	switch strings.ToLower(strings.TrimSpace(p.Action)) {
	case "start":
		if err := mgr.Start(sid); err != nil {
			return "", err
		}
		return fmt.Sprintf("Browser started (session=%s, headless background mode).", sid), nil
	case "stop":
		if err := mgr.Stop(sid); err != nil {
			return "", err
		}
		return "Browser stopped.", nil
	case "health":
		h := mgr.SessionHealth(sid)
		b, _ := json.Marshal(h)
		return string(b), nil
	case "list_sessions":
		sessions := mgr.ListSessions()
		b, _ := json.Marshal(sessions)
		return string(b), nil
	case "navigate":
		if strings.TrimSpace(p.URL) == "" {
			return "", fmt.Errorf("url is required for navigate")
		}
		u, err := mgr.Navigate(sid, p.URL)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Navigated to %s", u), nil
	case "snapshot":
		res, err := mgr.Snapshot(sid)
		if err != nil {
			return "", err
		}
		// Attach screenshot as AI-readable reference
		var out strings.Builder
		out.WriteString(fmt.Sprintf("Page: %s\nTitle: %s\nElements: %d\n\nDOM Snapshot:\n%s",
			res.URL, res.Title, res.ElementCount, res.DOMSnapshot))
		if capabilities.VisionEnabled() && res.Screenshot != "" {
			out.WriteString("\n\nScreenshot available for vision analysis.")
		}
		return out.String(), nil
	case "screenshot":
		png, err := mgr.Screenshot(sid)
		if err != nil {
			return "", err
		}
		rel, err := control.SaveImageBytes("image/png", png)
		if err != nil {
			return "", err
		}
		out := fmt.Sprintf("Screenshot saved to @%s (%d bytes).", rel, len(png))
		if capabilities.VisionEnabled() {
			br := capabilities.VisionBridge()
			if br != nil {
				if note, err := br.Analyze(ctx, rel, "browser screenshot"); err == nil {
					out += "\n\n" + vision.WrapNote(note)
				}
			}
		}
		return out, nil
	case "click":
		if strings.TrimSpace(p.Ref) == "" {
			return "", fmt.Errorf("ref is required for click")
		}
		if err := mgr.Click(sid, p.Ref); err != nil {
			return "", err
		}
		return fmt.Sprintf("Clicked %s", p.Ref), nil
	case "type":
		if strings.TrimSpace(p.Text) == "" {
			return "", fmt.Errorf("text is required for type")
		}
		if err := mgr.Type(sid, p.Ref, p.Text); err != nil {
			return "", err
		}
		return "Typed text.", nil
	case "scroll":
		dy := p.DeltaY
		if dy == 0 {
			dy = 400
		}
		if err := mgr.Scroll(sid, dy); err != nil {
			return "", err
		}
		return fmt.Sprintf("Scrolled by %.0f px.", dy), nil
	case "evaluate":
		if strings.TrimSpace(p.Expression) == "" {
			return "", fmt.Errorf("expression is required for evaluate")
		}
		out, err := mgr.Evaluate(sid, p.Expression)
		if err != nil {
			return "", err
		}
		return out, nil
	case "wait":
		ms := p.Milliseconds
		if ms <= 0 {
			ms = 1000
		}
		if err := mgr.Wait(sid, ms); err != nil {
			return "", err
		}
		return fmt.Sprintf("Waited %d ms.", ms), nil
	case "search":
		if strings.TrimSpace(p.URL) == "" {
			return "", fmt.Errorf("url is required for search")
		}
		maxChars := p.MaxChars
		if maxChars <= 0 {
			maxChars = 8000
		}
		res, err := mgr.WebSearch(ctx, p.URL, maxChars)
		if err != nil {
			return "", err
		}
		out := fmt.Sprintf("Title: %s\nURL: %s\n\n%s", res.Title, res.URL, res.Content)
		if len(res.Content) >= maxChars {
			out += "\n\n[content truncated]"
		}
		return out, nil
	default:
		return "", fmt.Errorf("unknown action %q", p.Action)
	}
}
