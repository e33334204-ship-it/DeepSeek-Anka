// Package computer provides desktop automation for the computer built-in tool,
// ported from openhanako's ComputerHost (Windows UIA / macOS CUA providers).
package computer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"reasonix/internal/proc"
)

// ─── Types ───────────────────────────────────────────────────────────────────

// UIAElement describes one UIA automation element from the accessibility tree.
type UIAElement struct {
	ElementID        string  `json:"elementId"`
	Role             string  `json:"role"`
	Label            string  `json:"label,omitempty"`
	Value            any     `json:"value,omitempty"`
	Enabled          bool    `json:"enabled"`
	Bounds           *Rect   `json:"bounds,omitempty"`
	Patterns         []string `json:"patterns,omitempty"`
	AutomationID     string  `json:"automationId,omitempty"`
	NativeWindowHndl int     `json:"nativeWindowHandle,omitempty"`
}

// Rect is a bounding rectangle.
type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// UIWindow describes one top-level application window.
type UIWindow struct {
	WindowID   string            `json:"windowId"`
	Title      string            `json:"title"`
	Bounds     *Rect             `json:"bounds,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// UIApp is an application with visible windows.
type UIApp struct {
	AppID        string    `json:"appId"`
	Name         string    `json:"name"`
	PID          int       `json:"pid,omitempty"`
	Windows      []UIWindow `json:"windows,omitempty"`
}

// AppState is the full snapshot for one application.
type AppState struct {
	AppID           string       `json:"appId"`
	WindowID        string       `json:"windowId"`
	ScreenshotPNG   []byte       `json:"-"`
	ScreenshotB64   string       `json:"screenshot,omitempty"`
	DisplayWidth    int          `json:"displayWidth"`
	DisplayHeight   int          `json:"displayHeight"`
	ScaleFactor     float64      `json:"scaleFactor"`
	FocusedElementID string      `json:"focusedElementId,omitempty"`
	Elements        []UIAElement `json:"elements"`
}

// PerformActionResult describes the outcome of a UI action.
type PerformActionResult struct {
	OK      bool   `json:"ok"`
	Mode    string `json:"mode,omitempty"`   // "background" or "foreground"
	Pattern string `json:"pattern,omitempty"` // which UIA pattern was used
}

// ─── Host ────────────────────────────────────────────────────────────────────

// Host orchestrates desktop control for one active lease at a time.
type Host struct {
	allowInput bool

	mu        sync.Mutex
	lease     *lease
	uiaHelper string // persisted helper script file path
	tmpDir    string
}

type lease struct {
	appID      string
	appName    string
	processID  int
	windowID   int
	windowHndl int
	started    time.Time
}

// NewHost creates a computer-use host.
func NewHost(allowWindowsInput bool) *Host {
	return &Host{
		allowInput: allowWindowsInput,
		tmpDir:     filepath.Join(os.TempDir(), "reasonix-computer-use"),
	}
}

// Enabled reports whether computer use is supported on this platform.
func Enabled() bool {
	return runtime.GOOS == "windows" || runtime.GOOS == "darwin"
}

// Status returns platform capability summary.
func (h *Host) Status() map[string]any {
	h.mu.Lock()
	lease := h.lease
	h.mu.Unlock()
	return map[string]any{
		"platform":     runtime.GOOS,
		"supported":    Enabled(),
		"inputAllowed": h.allowInput,
		"activeLease":  h.activeLeaseLocked(lease),
	}
}

func (h *Host) activeLeaseLocked(l *lease) any {
	if l == nil {
		return nil
	}
	return map[string]any{
		"appId":   l.appID,
		"appName": l.appName,
		"since":   l.started.Format(time.RFC3339),
	}
}

// ─── App listing ─────────────────────────────────────────────────────────────

// UIAAppInfo is a lightweight app descriptor returned by ListApps.
type UIAAppInfo struct {
	AppID     string `json:"appId"`
	Name      string `json:"name"`
	Title     string `json:"title,omitempty"`
	PID       int    `json:"pid,omitempty"`
	WindowID  string `json:"windowId,omitempty"`
}

// ListApps returns running applications with visible windows.
func (h *Host) ListApps(ctx context.Context) ([]UIAAppInfo, error) {
	switch runtime.GOOS {
	case "windows":
		return h.listAppsWindows(ctx)
	case "darwin":
		return h.listAppsDarwin(ctx)
	default:
		return nil, fmt.Errorf("computer use is not supported on %s yet", runtime.GOOS)
	}
}

func (h *Host) listAppsWindows(ctx context.Context) ([]UIAAppInfo, error) {
	raw, err := h.runHelper(ctx, map[string]any{"command": "list_apps"})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Apps []struct {
			AppID     string `json:"appId"`
			Name      string `json:"name"`
			ProcessID int    `json:"processId"`
			Windows   []struct {
				WindowID string `json:"windowId"`
				Title    string `json:"title"`
			} `json:"windows"`
		} `json:"apps"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parse list_apps response: %w", err)
	}
	out := make([]UIAAppInfo, 0, len(resp.Apps))
	for _, app := range resp.Apps {
		for _, win := range app.Windows {
			out = append(out, UIAAppInfo{
				AppID:    app.AppID,
				Name:     app.Name,
				Title:    win.Title,
				PID:      app.ProcessID,
				WindowID: win.WindowID,
			})
		}
	}
	return out, nil
}

func (h *Host) listAppsDarwin(ctx context.Context) ([]UIAAppInfo, error) {
	script := `tell application "System Events"
set procs to every application process whose background only is false
set out to ""
repeat with p in procs
try
set out to out & (unix id of p as text) & tab & (name of p as text) & linefeed
end try
end repeat
return out
end tell`
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	proc.HideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var apps []UIAAppInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "\t", 2)
		if len(parts) != 2 {
			continue
		}
		app := UIAAppInfo{AppID: "pid:" + parts[0], Name: parts[1], PID: parseInt(parts[0])}
		if app.PID > 0 {
			apps = append(apps, app)
		}
	}
	return apps, nil
}

func parseInt(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

// ─── Lease management ────────────────────────────────────────────────────────

// Start begins control of an application by id, name, or PID.
func (h *Host) Start(appID, appName string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lease = &lease{appID: appID, appName: appName, started: time.Now()}
	return nil
}

// Stop ends the active lease.
func (h *Host) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lease = nil
}

// ─── App state (screenshot + UIA tree) ────────────────────────────────────────

// GetAppState captures a screenshot and the UIA accessibility tree for the
// active lease window. On Windows, this returns a full element tree; on macOS
// it returns just the screenshot.
func (h *Host) GetAppState(ctx context.Context) (*AppState, error) {
	h.mu.Lock()
	l := h.lease
	h.mu.Unlock()

	switch runtime.GOOS {
	case "windows":
		return h.getAppStateWindows(ctx, l)
	case "darwin":
		return h.getAppStateDarwin(ctx, l)
	default:
		return nil, fmt.Errorf("computer use is not supported on %s", runtime.GOOS)
	}
}

func (h *Host) getAppStateWindows(ctx context.Context, l *lease) (*AppState, error) {
	target := map[string]any{}
	if l != nil {
		if l.processID > 0 {
			target["processId"] = l.processID
		}
		if l.windowID > 0 {
			target["windowId"] = l.windowID
		}
		if l.appName != "" {
			target["appName"] = l.appName
		}
	}
	raw, err := h.runHelper(ctx, map[string]any{
		"command": "get_app_state",
		"target":  target,
	})
	if err != nil {
		return nil, err
	}
	var resp struct {
		AppID    string `json:"appId"`
		WindowID string `json:"windowId"`
		Screenshot *struct {
			Data        string  `json:"data"` // base64
			MimeType    string  `json:"mimeType"`
			Width       int     `json:"width"`
			Height      int     `json:"height"`
			ScaleFactor float64 `json:"scaleFactor"`
		} `json:"screenshot"`
		Display *struct {
			Width       int     `json:"width"`
			Height      int     `json:"height"`
			ScaleFactor float64 `json:"scaleFactor"`
			X           int     `json:"x"`
			Y           int     `json:"y"`
		} `json:"display"`
		Elements []UIAElement `json:"elements"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parse get_app_state: %w", err)
	}

	state := &AppState{
		AppID:    resp.AppID,
		WindowID: resp.WindowID,
		Elements: resp.Elements,
	}
	if resp.Screenshot != nil {
		state.ScreenshotB64 = resp.Screenshot.Data
		state.ScaleFactor = resp.Screenshot.ScaleFactor
		png, decErr := base64.StdEncoding.DecodeString(resp.Screenshot.Data)
		if decErr == nil {
			state.ScreenshotPNG = png
		}
		if resp.Screenshot.Width > 0 {
			state.DisplayWidth = resp.Screenshot.Width
		}
		if resp.Screenshot.Height > 0 {
			state.DisplayHeight = resp.Screenshot.Height
		}
	}
	if resp.Display != nil {
		if resp.Display.Width > 0 {
			state.DisplayWidth = resp.Display.Width
		}
		if resp.Display.Height > 0 {
			state.DisplayHeight = resp.Display.Height
		}
		if resp.Display.ScaleFactor > 0 {
			state.ScaleFactor = resp.Display.ScaleFactor
		}
	}
	return state, nil
}

func (h *Host) getAppStateDarwin(ctx context.Context, l *lease) (*AppState, error) {
	png, err := h.captureScreen(ctx)
	if err != nil {
		return nil, err
	}
	state := &AppState{
		ScreenshotPNG: png,
		ScreenshotB64: base64.StdEncoding.EncodeToString(png),
	}
	if l != nil {
		state.AppID = l.appID
	}
	return state, nil
}

// ─── Actions ─────────────────────────────────────────────────────────────────

// ClickElement clicks a UI element by its elementId from a recent snapshot.
func (h *Host) ClickElement(ctx context.Context, elementID string, snapshotElement *UIAElement) (*PerformActionResult, error) {
	return h.performAction(ctx, "click_element", elementID, map[string]any{
		"snapshotElement": snapshotElement,
	})
}

// DoubleClick double-clicks a UI element.
func (h *Host) DoubleClick(ctx context.Context, elementID string, snapshotElement *UIAElement) (*PerformActionResult, error) {
	return h.performAction(ctx, "double_click", elementID, map[string]any{
		"snapshotElement": snapshotElement,
	})
}

// TypeText types text into a UI element.
func (h *Host) TypeText(ctx context.Context, elementID, text string, snapshotElement *UIAElement) (*PerformActionResult, error) {
	return h.performAction(ctx, "type_text", elementID, map[string]any{
		"text":           text,
		"snapshotElement": snapshotElement,
	})
}

// PressKey sends a key chord.
func (h *Host) PressKey(ctx context.Context, key string) (*PerformActionResult, error) {
	return h.performAction(ctx, "press_key", "", map[string]any{
		"key": key,
	})
}

// Scroll scrolls a UI element in a direction.
func (h *Host) Scroll(ctx context.Context, elementID, direction string, amount int, snapshotElement *UIAElement) (*PerformActionResult, error) {
	return h.performAction(ctx, "scroll", elementID, map[string]any{
		"direction":      direction,
		"amount":         amount,
		"snapshotElement": snapshotElement,
	})
}

// ClickPoint clicks at absolute screen coordinates.
func (h *Host) ClickPoint(ctx context.Context, x, y int) (*PerformActionResult, error) {
	return h.performAction(ctx, "click_point", "", map[string]any{
		"x": x, "y": y,
	})
}

func (h *Host) performAction(ctx context.Context, actionType, elementID string, extra map[string]any) (*PerformActionResult, error) {
	if !h.allowInput {
		return nil, fmt.Errorf("input injection is disabled; set computer.allow_windows_input_injection = true")
	}
	switch runtime.GOOS {
	case "windows":
		return h.performActionWindows(ctx, actionType, elementID, extra)
	default:
		return nil, fmt.Errorf("action %q is not implemented on %s", actionType, runtime.GOOS)
	}
}

func (h *Host) performActionWindows(ctx context.Context, actionType, elementID string, extra map[string]any) (*PerformActionResult, error) {
	payload := map[string]any{
		"command": "perform_action",
		"action": map[string]any{
			"type":      actionType,
			"elementId": elementID,
		},
	}
	// Merge extra keys into the action object
	if action, ok := payload["action"].(map[string]any); ok {
		for k, v := range extra {
			if v != nil {
				action[k] = v
			}
		}
	}
	// Add target from current lease
	h.mu.Lock()
	l := h.lease
	h.mu.Unlock()
	if l != nil {
		target := map[string]any{}
		if l.processID > 0 {
			target["processId"] = l.processID
		}
		if l.windowID > 0 {
			target["windowId"] = l.windowID
		}
		if l.appName != "" {
			target["appName"] = l.appName
		}
		payload["target"] = target
	}

	raw, err := h.runHelper(ctx, payload)
	if err != nil {
		return nil, err
	}
	var result PerformActionResult
	if err := json.Unmarshal(raw, &result); err != nil {
		// OK may still be set even without other fields
		if strings.Contains(string(raw), `"ok":true`) {
			return &PerformActionResult{OK: true}, nil
		}
		return nil, fmt.Errorf("parse action response: %w (%s)", err, strings.TrimSpace(string(raw)))
	}
	if !result.OK {
		return &result, fmt.Errorf("action %q failed", actionType)
	}
	return &result, nil
}

// ─── Helper runner ───────────────────────────────────────────────────────────

// helperRequest is the JSON sent to the UIA helper script.
type helperRequest struct {
	Command string         `json:"command"`
	Target  map[string]any `json:"target,omitempty"`
	Action  map[string]any `json:"action,omitempty"`
}

// helperResponse is the JSON envelope from the UIA helper.
type helperResponse struct {
	OK      bool              `json:"ok"`
	Error   string            `json:"error,omitempty"`
	ErrorCode string          `json:"errorCode,omitempty"`
	Message string            `json:"message,omitempty"`
	Data    json.RawMessage   `json:"data,omitempty"`
	Details map[string]any    `json:"details,omitempty"`
}

// runHelper sends a request to the Windows UIA PowerShell helper and returns
// the parsed data payload (the "data" field of the response). On error it
// returns a descriptive error.
func (h *Host) runHelper(ctx context.Context, payload map[string]any) (json.RawMessage, error) {
	_ = os.MkdirAll(h.tmpDir, 0o755)

	// Ensure the helper script is written (with content-hash naming).
	scriptContent := windowsUIAScript()
	scriptHash := sha256Hex(scriptContent)
	helperName := fmt.Sprintf("reasonix-uia-helper-%s.ps1", scriptHash[:16])
	helperPath := filepath.Join(h.tmpDir, helperName)
	if _, err := os.Stat(helperPath); os.IsNotExist(err) {
		if err := os.WriteFile(helperPath, []byte(scriptContent), 0o644); err != nil {
			return nil, fmt.Errorf("write UIA helper: %w", err)
		}
	}

	// Generate unique request/result file paths.
	id := fmt.Sprintf("%d-%d-%x", os.Getpid(), time.Now().UnixNano(), time.Now().UnixNano()&0xffff)
	reqPath := filepath.Join(h.tmpDir, fmt.Sprintf("uia-req-%s.json", id))
	resPath := filepath.Join(h.tmpDir, fmt.Sprintf("uia-res-%s.json", id))
	defer os.Remove(reqPath)
	defer os.Remove(resPath)

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal helper request: %w", err)
	}
	if err := os.WriteFile(reqPath, body, 0o644); err != nil {
		return nil, fmt.Errorf("write helper request: %w", err)
	}

	cmd := exec.CommandContext(ctx, "powershell",
		"-NoLogo", "-NoProfile", "-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-File", helperPath,
		"-RequestPath", reqPath,
		"-ResultPath", resPath,
	)
	proc.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("UIA helper failed to run: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}

	// Read the result file.
	resBody, err := os.ReadFile(resPath)
	if err != nil {
		return nil, fmt.Errorf("UIA helper result file not found: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	var resp helperResponse
	if err := json.Unmarshal(resBody, &resp); err != nil {
		return nil, fmt.Errorf("parse UIA response: %w (body: %s)", err, strings.TrimSpace(string(resBody)))
	}
	if !resp.OK {
		msg := resp.Message
		if msg == "" {
			msg = resp.Error
		}
		if msg == "" {
			msg = "UIA helper returned error"
		}
		return nil, fmt.Errorf("UIA: %s (code=%s)", msg, resp.ErrorCode)
	}
	return resp.Data, nil
}

// captureScreen captures the full desktop screenshot.
func (h *Host) captureScreen(ctx context.Context) ([]byte, error) {
	switch runtime.GOOS {
	case "windows":
		script := `Add-Type -AssemblyName System.Windows.Forms,System.Drawing
$bounds = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
$bmp = New-Object System.Drawing.Bitmap $bounds.Width, $bounds.Height
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($bounds.Location, [System.Drawing.Point]::Empty, $bounds.Size)
$ms = New-Object System.IO.MemoryStream
$bmp.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
[Convert]::ToBase64String($ms.ToArray())`
		cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
		proc.HideWindow(cmd)
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("screenshot: %w", err)
		}
		return base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	case "darwin":
		tmp := filepath.Join(h.tmpDir, fmt.Sprintf("shot-%d.png", time.Now().UnixNano()))
		_ = os.MkdirAll(h.tmpDir, 0o755)
		cmd := exec.CommandContext(ctx, "screencapture", "-x", tmp)
		if err := cmd.Run(); err != nil {
			return nil, err
		}
		defer os.Remove(tmp)
		return os.ReadFile(tmp)
	default:
		return nil, fmt.Errorf("screenshot not supported on %s", runtime.GOOS)
	}
}

// sha256Hex returns a hex hash of s for file naming.
func sha256Hex(s string) string {
	return fmt.Sprintf("%x", len(s))
}
