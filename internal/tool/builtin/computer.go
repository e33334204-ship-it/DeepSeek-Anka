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

func init() { tool.RegisterBuiltin(computerTool{}) }

type computerTool struct{}

func (computerTool) Name() string { return "computer" }

func (computerTool) Description() string {
	return "Control the desktop: list apps, capture screen state, click elements, type text, and press keys. Requires [computer].enabled = true."
}

func (computerTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "action":{"type":"string","enum":["status","list_apps","start","get_app_state","click_element","double_click","type_text","press_key","scroll","click_point","stop"],"description":"Computer control action"},
  "app_id":{"type":"string","description":"Application id from list_apps"},
  "app_name":{"type":"string","description":"Application display name"},
  "element_id":{"type":"string","description":"UI element id from get_app_state elements"},
  "text":{"type":"string","description":"Text to type"},
  "key":{"type":"string","description":"Key to press (Enter, Tab, Escape, etc.)"},
  "direction":{"type":"string","enum":["up","down","left","right"],"description":"Scroll direction"},
  "x":{"type":"integer","description":"X coordinate for click_point"},
  "y":{"type":"integer","description":"Y coordinate for click_point"}
},
"required":["action"]
}`)
}

func (computerTool) ReadOnly() bool { return false }

func (computerTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	host := capabilities.ComputerHost()
	if host == nil {
		return "", fmt.Errorf("computer use is disabled; enable [computer] in reasonix.toml")
	}
	var p struct {
		Action    string `json:"action"`
		AppID     string `json:"app_id"`
		AppName   string `json:"app_name"`
		ElementID string `json:"element_id"`
		Text      string `json:"text"`
		Key       string `json:"key"`
		Direction string `json:"direction"`
		X         int    `json:"x"`
		Y         int    `json:"y"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(p.Action)) {
	case "status":
		b, _ := json.Marshal(host.Status())
		return string(b), nil
	case "list_apps":
		apps, err := host.ListApps(ctx)
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(apps)
		return string(b), nil
	case "start":
		if strings.TrimSpace(p.AppID) == "" && strings.TrimSpace(p.AppName) == "" {
			return "", fmt.Errorf("app_id or app_name is required")
		}
		if err := host.Start(p.AppID, p.AppName); err != nil {
			return "", err
		}
		return fmt.Sprintf("Started computer lease for %s (%s).", p.AppName, p.AppID), nil
	case "get_app_state":
		state, err := host.GetAppState(ctx)
		if err != nil {
			return "", err
		}
		rel, err := control.SaveImageBytes("image/png", state.ScreenshotPNG)
		if err != nil {
			return "", err
		}
		out := fmt.Sprintf("App state captured: %s (window %s)\n%d UI elements\nScreenshot: @%s (%d bytes).",
			state.AppID, state.WindowID, len(state.Elements), rel, len(state.ScreenshotPNG))
		if state.ScaleFactor > 0 {
			out += fmt.Sprintf("\nDisplay: %dx%d (scale %.1f)", state.DisplayWidth, state.DisplayHeight, state.ScaleFactor)
		}
		if capabilities.VisionEnabled() {
			if br := capabilities.VisionBridge(); br != nil {
				if note, err := br.Analyze(ctx, rel, "desktop app state"); err == nil {
					out += "\n\n" + vision.WrapNote(note)
				}
			}
		}
		return out, nil
	case "click_element":
		if strings.TrimSpace(p.ElementID) == "" {
			return "", fmt.Errorf("element_id is required")
		}
		res, err := host.ClickElement(ctx, p.ElementID, nil)
		if err != nil {
			return "", err
		}
		out := "Clicked element."
		if res != nil && res.Pattern != "" {
			out += fmt.Sprintf(" (via %s)", res.Pattern)
		}
		return out, nil
	case "double_click":
		if strings.TrimSpace(p.ElementID) == "" {
			return "", fmt.Errorf("element_id is required")
		}
		res, err := host.DoubleClick(ctx, p.ElementID, nil)
		if err != nil {
			return "", err
		}
		out := "Double-clicked element."
		if res != nil && res.Pattern != "" {
			out += fmt.Sprintf(" (via %s)", res.Pattern)
		}
		return out, nil
	case "type_text":
		if strings.TrimSpace(p.Text) == "" {
			return "", fmt.Errorf("text is required")
		}
		res, err := host.TypeText(ctx, p.ElementID, p.Text, nil)
		if err != nil {
			return "", err
		}
		out := "Typed text."
		if res != nil && res.Pattern != "" {
			out += fmt.Sprintf(" (via %s)", res.Pattern)
		}
		return out, nil
	case "press_key":
		if strings.TrimSpace(p.Key) == "" {
			return "", fmt.Errorf("key is required")
		}
		res, err := host.PressKey(ctx, p.Key)
		if err != nil {
			return "", err
		}
		_ = res
		return "Sent key.", nil
	case "scroll":
		if strings.TrimSpace(p.Direction) == "" {
			return "", fmt.Errorf("direction is required")
		}
		res, err := host.Scroll(ctx, p.ElementID, p.Direction, 1, nil)
		if err != nil {
			return "", err
		}
		out := fmt.Sprintf("Scrolled %s.", p.Direction)
		if res != nil && res.Pattern != "" {
			out += fmt.Sprintf(" (via %s)", res.Pattern)
		}
		return out, nil
	case "click_point":
		res, err := host.ClickPoint(ctx, p.X, p.Y)
		if err != nil {
			return "", err
		}
		_ = res
		return fmt.Sprintf("Clicked at (%d, %d).", p.X, p.Y), nil
	case "stop":
		host.Stop()
		return "Computer lease stopped.", nil
	default:
		return "", fmt.Errorf("unknown action %q", p.Action)
	}
}
