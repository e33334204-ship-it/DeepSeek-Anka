package builtin

import (
	"context"
	"fmt"

	"deepseek-anka/internal/capabilities"
	"deepseek-anka/internal/control"
)

// SaveDesktopScreenshot captures the primary display and stores it as an attachment.
func SaveDesktopScreenshot(ctx context.Context) (string, error) {
	host := capabilities.ComputerHost()
	if host == nil {
		return "", fmt.Errorf("computer host unavailable")
	}
	state, err := host.GetAppState(ctx)
	if err != nil {
		return "", err
	}
	return control.SaveImageBytes("image/png", state.ScreenshotPNG)
}
