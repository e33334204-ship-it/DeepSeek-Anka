package builtin

import (
	"context"
	"encoding/json"
	"fmt"

	"reasonix/internal/capabilities"
	"reasonix/internal/tool"
	"reasonix/internal/vision"
)

func init() { tool.RegisterBuiltin(screenshot{}) }

type screenshot struct{}

func (screenshot) Name() string { return "screenshot" }

func (screenshot) Description() string {
	return "Capture a screenshot of the primary display and save it under .reasonix/attachments. When vision auxiliary is enabled, a textual description is included."
}

func (screenshot) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func (screenshot) ReadOnly() bool { return true }

func (screenshot) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	host := capabilities.ComputerHost()
	if host == nil {
		return "", fmt.Errorf("screenshot requires [computer].enabled = true")
	}
	rel, err := SaveDesktopScreenshot(ctx)
	if err != nil {
		return "", err
	}
	out := fmt.Sprintf("Screenshot saved to @%s.", rel)
	if capabilities.VisionEnabled() {
		if br := capabilities.VisionBridge(); br != nil {
			if note, err := br.Analyze(ctx, rel, "desktop screenshot"); err == nil {
				out += "\n\n" + vision.WrapNote(note)
			}
		}
	}
	return out, nil
}
