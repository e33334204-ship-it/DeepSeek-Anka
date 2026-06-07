// Package browser provides a headless Chromium session manager for the browser
// built-in tool, ported from openhanako's BrowserManager (Electron webContents).
package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

const (
	maxInstances                = 5
	defaultViewportWidth        = 1280
	defaultViewportHeight       = 900
	maxFullPageScreenshotWidth  = 4000
	maxFullPageScreenshotHeight = 12000
)

// SessionState tracks a browser session's lifecycle.
type SessionState string

const (
	SessionRunning   SessionState = "running"
	SessionSuspended SessionState = "suspended"
	SessionUnhealthy SessionState = "unhealthy"
)

// SessionInfo is a snapshot of a browser session's status.
type SessionInfo struct {
	ID                string       `json:"id"`
	URL               string       `json:"url"`
	State             SessionState `json:"state"`
	UnavailableReason string       `json:"unavailableReason,omitempty"`
}

// SnapshotResult is the rich output of a snapshot action.
type SnapshotResult struct {
	URL          string `json:"url"`
	Title        string `json:"title"`
	DOMSnapshot  string `json:"domSnapshot"`
	Screenshot   string `json:"screenshot,omitempty"` // data:image/png;base64,...
	ElementCount int    `json:"elementCount"`
}

// SearchResult is the output of a web search action.
type SearchResult struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	Content string `json:"content,omitempty"` // extracted body text
}

// Manager owns per-session Chromium instances.
type Manager struct {
	headless  bool
	chrome    string
	coldState string // path to cold-state JSON file

	mu       sync.Mutex
	sessions map[string]*sessionState
	order    []string
}

type sessionState struct {
	cancel  context.CancelFunc
	ctx     context.Context
	url     string
	health  SessionState
	err     string // unavailable reason
	started time.Time
	refs    map[cdp.NodeID]string // node id -> ref label, populated by Snapshot
}

// coldState is the on-disk format for URL persistence.
type coldState struct {
	Sessions map[string]coldSession `json:"sessions"`
}

type coldSession struct {
	URL string `json:"url"`
}

// NewManager creates a browser manager.
func NewManager(headless bool, chromePath string) *Manager {
	return &Manager{
		headless:  headless,
		chrome:    chromePath,
		coldState: filepath.Join(os.TempDir(), "reasonix-browser-coldstate.json"),
		sessions:  map[string]*sessionState{},
	}
}

// ColdStatePath sets a custom path for the cold-state file. Used in testing.
func (m *Manager) ColdStatePath(p string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.coldState = p
}

// Close shuts down all browser sessions.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.sessions {
		if s.cancel != nil {
			s.cancel()
		}
		delete(m.sessions, id)
	}
	m.order = nil
}

func (m *Manager) getSession(id string) (*sessionState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("browser session %q is not running; call action=start first", id)
	}
	if s.health == SessionUnhealthy {
		r := s.err
		if r == "" {
			r = "session is unavailable"
		}
		return nil, fmt.Errorf("browser session %q is unavailable: %s", id, r)
	}
	return s, nil
}

func (m *Manager) allocOpts() []chromedp.ExecAllocatorOption {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.WindowSize(defaultViewportWidth, defaultViewportHeight),
	)
	if m.headless {
		opts = append(opts, chromedp.Flag("headless", true))
	}
	if m.chrome != "" {
		opts = append(opts, chromedp.ExecPath(m.chrome))
	}
	return opts
}

// ─── Lifecycle ───────────────────────────────────────────────────────────────

// Start launches a browser for sessionID.
func (m *Manager) Start(sessionID string) error {
	m.mu.Lock()
	// Already running.
	if s, ok := m.sessions[sessionID]; ok && s.health == SessionRunning {
		m.touchLocked(sessionID)
		m.mu.Unlock()
		return nil
	}
	// Unhealthy: replace.
	if s, ok := m.sessions[sessionID]; ok && s.health == SessionUnhealthy {
		if s.cancel != nil {
			s.cancel()
		}
		delete(m.sessions, sessionID)
	}
	m.evictIfNeededLocked()

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), m.allocOpts()...)
	browserCtx, cancel := chromedp.NewContext(allocCtx)
	st := &sessionState{
		cancel:  func() { cancel(); allocCancel() },
		ctx:     browserCtx,
		health:  SessionRunning,
		started: time.Now(),
		refs:    map[cdp.NodeID]string{},
	}
	m.sessions[sessionID] = st
	m.touchLocked(sessionID)
	m.mu.Unlock()

	if err := chromedp.Run(browserCtx, chromedp.Navigate("about:blank")); err != nil {
		m.markUnhealthy(sessionID, err.Error())
		return fmt.Errorf("browser start: %w", err)
	}

	// Try to restore a cold-saved URL.
	if saved := m.loadColdURL(sessionID); saved != "" {
		if navURL, err := m.Navigate(sessionID, saved); err == nil {
			st.url = navURL
		}
	}
	return nil
}

func (m *Manager) touchLocked(id string) {
	for i, v := range m.order {
		if v == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			return
		}
	}
	m.order = append(m.order, id)
}

func (m *Manager) evictIfNeededLocked() {
	for len(m.sessions) >= maxInstances && len(m.order) > 0 {
		oldest := m.order[0]
		m.order = m.order[1:]
		if s, ok := m.sessions[oldest]; ok {
			if s.cancel != nil {
				s.cancel()
			}
			delete(m.sessions, oldest)
		}
	}
}

// Stop closes a browser session.
func (m *Manager) Stop(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sessionID]
	if !ok {
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	delete(m.sessions, sessionID)
	m.removeColdURL(sessionID)
	for i, v := range m.order {
		if v == sessionID {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	return nil
}

// ─── Health ──────────────────────────────────────────────────────────────────

// markUnhealthy sets a session as unhealthy with a reason.
func (m *Manager) markUnhealthy(sessionID, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[sessionID]; ok {
		s.health = SessionUnhealthy
		s.err = reason
	}
}

// SessionHealth returns the health status for a session. Does not error on
// missing session — returns a suspended/missing info instead.
func (m *Manager) SessionHealth(sessionID string) SessionInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[sessionID]; ok {
		return SessionInfo{
			ID:                sessionID,
			URL:               s.url,
			State:             s.health,
			UnavailableReason: s.err,
		}
	}
	// Check cold state.
	if url := m.loadColdURL(sessionID); url != "" {
		return SessionInfo{
			ID:    sessionID,
			URL:   url,
			State: SessionSuspended,
		}
	}
	return SessionInfo{ID: sessionID, State: SessionSuspended}
}

// ListSessions returns all known browser sessions (active + cold-saved).
func (m *Manager) ListSessions() []SessionInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	seen := map[string]bool{}
	var out []SessionInfo
	for id, s := range m.sessions {
		seen[id] = true
		out = append(out, SessionInfo{
			ID:                id,
			URL:               s.url,
			State:             s.health,
			UnavailableReason: s.err,
		})
	}

	// Add cold-saved sessions not in memory.
	cold := m.loadColdStateLocked()
	for id, cs := range cold.Sessions {
		if seen[id] {
			continue
		}
		out = append(out, SessionInfo{
			ID:    id,
			URL:   cs.URL,
			State: SessionSuspended,
		})
	}
	return out
}

// ─── Cold state persistence ─────────────────────────────────────────────────

func (m *Manager) loadColdStateLocked() coldState {
	data, err := os.ReadFile(m.coldState)
	if err != nil {
		return coldState{Sessions: map[string]coldSession{}}
	}
	var cs coldState
	if err := json.Unmarshal(data, &cs); err != nil {
		return coldState{Sessions: map[string]coldSession{}}
	}
	if cs.Sessions == nil {
		cs.Sessions = map[string]coldSession{}
	}
	return cs
}

func (m *Manager) saveColdStateLocked(cs coldState) {
	if cs.Sessions == nil {
		cs.Sessions = map[string]coldSession{}
	}
	dir := filepath.Dir(m.coldState)
	_ = os.MkdirAll(dir, 0o755)
	data, _ := json.Marshal(cs)
	_ = os.WriteFile(m.coldState, data, 0o644)
}

func (m *Manager) loadColdURL(sessionID string) string {
	cs := m.loadColdStateLocked()
	if s, ok := cs.Sessions[sessionID]; ok {
		return s.URL
	}
	return ""
}

func (m *Manager) saveColdURL(sessionID, url string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cs := m.loadColdStateLocked()
	cs.Sessions[sessionID] = coldSession{URL: url}
	m.saveColdStateLocked(cs)
}

func (m *Manager) removeColdURL(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cs := m.loadColdStateLocked()
	delete(cs.Sessions, sessionID)
	m.saveColdStateLocked(cs)
}

// ─── Navigation ──────────────────────────────────────────────────────────────

// Navigate opens a URL.
func (m *Manager) Navigate(sessionID, url string) (string, error) {
	s, err := m.getSession(sessionID)
	if err != nil {
		return "", err
	}
	if err := chromedp.Run(s.ctx, chromedp.Navigate(url), chromedp.WaitReady("body", chromedp.ByQuery)); err != nil {
		return "", err
	}
	s.url = url
	m.saveColdURL(sessionID, url)
	return url, nil
}

// ─── Snapshot (rich) ─────────────────────────────────────────────────────────

// Snapshot returns a simplified accessibility tree of interactive elements
// combined with the page URL and title.
func (m *Manager) Snapshot(sessionID string) (*SnapshotResult, error) {
	s, err := m.getSession(sessionID)
	if err != nil {
		return nil, err
	}

	// Get page info (title, URL) and DOM tree in parallel.
	var title string
	var url string
	var root *cdp.Node
	if err := chromedp.Run(s.ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var t string
			if err := chromedp.Evaluate("document.title", &t).Do(ctx); err == nil {
				title = t
			}
			var u string
			if err := chromedp.Evaluate("document.URL", &u).Do(ctx); err == nil {
				url = u
			}
			return nil
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			root, err = dom.GetDocument().WithDepth(-1).Do(ctx)
			return err
		}),
	); err != nil {
		return nil, err
	}

	if url != "" {
		s.url = url
	}

	result := &SnapshotResult{URL: url, Title: title}
	if root == nil {
		result.DOMSnapshot = "(empty page)"
		return result, nil
	}

	s.refs = map[cdp.NodeID]string{}
	var b strings.Builder
	count := walkNodes(&b, root, s.refs, 0, 0)
	result.DOMSnapshot = b.String()
	result.ElementCount = count
	return result, nil
}

func walkNodes(b *strings.Builder, n *cdp.Node, refs map[cdp.NodeID]string, depth, count int) int {
	if n == nil || count > 200 {
		return count
	}
	tag := strings.ToLower(n.NodeName)
	interesting := tag == "a" || tag == "button" || tag == "input" || tag == "textarea" || tag == "select" || n.AttributeValue("role") != ""
	text := strings.TrimSpace(n.NodeValue)
	if interesting || (text != "" && len(text) < 120) {
		ref := fmt.Sprintf("ref-%d", n.NodeID)
		refs[n.NodeID] = ref
		indent := strings.Repeat("  ", depth)
		attrs := formatAttrs(n)
		fmt.Fprintf(b, "%s[%s id=%d ref=%s]%s %s\n", indent, tag, n.NodeID, ref, attrs, text)
		count++
	}
	for _, c := range n.Children {
		count = walkNodes(b, c, refs, depth+1, count)
	}
	return count
}

func formatAttrs(n *cdp.Node) string {
	var parts []string
	for i := 0; i+1 < len(n.Attributes); i += 2 {
		k, v := n.Attributes[i], n.Attributes[i+1]
		switch k {
		case "href", "type", "name", "placeholder", "aria-label", "value":
			parts = append(parts, fmt.Sprintf("%s=%q", k, truncateStr(v, 80)))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// SnapshotText is a compatibility wrapper that returns just the text DOM tree.
func (m *Manager) SnapshotText(sessionID string) (string, error) {
	r, err := m.Snapshot(sessionID)
	if err != nil {
		return "", err
	}
	return r.DOMSnapshot, nil
}

// ─── Screenshot ──────────────────────────────────────────────────────────────

// Screenshot captures the current page as PNG bytes. When fullPage is true it
// captures the scrollable page area, capped to avoid giant images.
func (m *Manager) Screenshot(sessionID string, fullPage bool) ([]byte, error) {
	s, err := m.getSession(sessionID)
	if err != nil {
		return nil, err
	}
	var buf []byte
	if err := chromedp.Run(s.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		if fullPage {
			buf, err = captureFullPageScreenshot(ctx)
			if err == nil && len(buf) > 0 {
				return nil
			}
		}
		buf, err = page.CaptureScreenshot().
			WithFormat(page.CaptureScreenshotFormatPng).
			WithOptimizeForSpeed(true).
			Do(ctx)
		return err
	})); err != nil {
		return nil, err
	}
	return buf, nil
}

func captureFullPageScreenshot(ctx context.Context) ([]byte, error) {
	_, _, _, _, visualViewport, contentSize, err := page.GetLayoutMetrics().Do(ctx)
	if err != nil {
		return nil, err
	}
	if contentSize == nil {
		return nil, fmt.Errorf("page content size unavailable")
	}
	width := math.Ceil(contentSize.Width)
	height := math.Ceil(contentSize.Height)
	if visualViewport != nil {
		width = math.Max(width, math.Ceil(visualViewport.ClientWidth))
		height = math.Max(height, math.Ceil(visualViewport.ClientHeight))
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid page content size %.0fx%.0f", width, height)
	}
	width = math.Min(width, maxFullPageScreenshotWidth)
	height = math.Min(height, maxFullPageScreenshotHeight)
	return page.CaptureScreenshot().
		WithFormat(page.CaptureScreenshotFormatPng).
		WithClip(&page.Viewport{X: 0, Y: 0, Width: width, Height: height, Scale: 1}).
		WithCaptureBeyondViewport(true).
		WithOptimizeForSpeed(true).
		Do(ctx)
}

// ScreenshotBase64 returns a data URL for model/tool output.
func (m *Manager) ScreenshotBase64(sessionID string) (string, error) {
	raw, err := m.Screenshot(sessionID, true)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw), nil
}

// ─── Interaction ─────────────────────────────────────────────────────────────

// Click clicks an element by ref from Snapshot.
func (m *Manager) Click(sessionID, ref string) error {
	s, err := m.getSession(sessionID)
	if err != nil {
		return err
	}
	nodeID, err := s.refNode(ref)
	if err != nil {
		return err
	}
	return chromedp.Run(s.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		boxes, err := dom.GetContentQuads().WithNodeID(nodeID).Do(ctx)
		if err != nil || len(boxes) == 0 {
			return fmt.Errorf("element %s not clickable", ref)
		}
		quad := boxes[0]
		x := (quad[0] + quad[2] + quad[4] + quad[6]) / 4
		y := (quad[1] + quad[3] + quad[5] + quad[7]) / 4
		return input.DispatchMouseEvent(input.MousePressed, x, y).
			WithButton(input.Left).
			WithClickCount(1).
			Do(ctx)
	}), chromedp.ActionFunc(func(ctx context.Context) error {
		boxes, _ := dom.GetContentQuads().WithNodeID(nodeID).Do(ctx)
		if len(boxes) == 0 {
			return nil
		}
		quad := boxes[0]
		x := (quad[0] + quad[2] + quad[4] + quad[6]) / 4
		y := (quad[1] + quad[3] + quad[5] + quad[7]) / 4
		return input.DispatchMouseEvent(input.MouseReleased, x, y).
			WithButton(input.Left).
			WithClickCount(1).
			Do(ctx)
	}))
}

func (s *sessionState) refNode(ref string) (cdp.NodeID, error) {
	for id, r := range s.refs {
		if r == ref {
			return id, nil
		}
	}
	return 0, fmt.Errorf("unknown ref %q; run snapshot first", ref)
}

// Type enters text into the focused element or by ref.
func (m *Manager) Type(sessionID, ref, text string) error {
	s, err := m.getSession(sessionID)
	if err != nil {
		return err
	}
	return chromedp.Run(s.ctx, chromedp.SendKeys("body", text, chromedp.ByQuery))
}

// Scroll scrolls the page.
func (m *Manager) Scroll(sessionID string, deltaY float64) error {
	s, err := m.getSession(sessionID)
	if err != nil {
		return err
	}
	return chromedp.Run(s.ctx, chromedp.Evaluate(fmt.Sprintf("window.scrollBy(0, %f)", deltaY), nil))
}

// Evaluate runs JavaScript and returns JSON-encoded result.
func (m *Manager) Evaluate(sessionID, expr string) (string, error) {
	s, err := m.getSession(sessionID)
	if err != nil {
		return "", err
	}
	var out any
	if err := chromedp.Run(s.ctx, chromedp.Evaluate(expr, &out)); err != nil {
		return "", err
	}
	b, err := json.Marshal(out)
	if err != nil {
		return fmt.Sprintf("%v", out), nil
	}
	return string(b), nil
}

// Wait sleeps for duration.
func (m *Manager) Wait(sessionID string, ms int) error {
	s, err := m.getSession(sessionID)
	if err != nil {
		return err
	}
	d := time.Duration(ms) * time.Millisecond
	return chromedp.Run(s.ctx, chromedp.Sleep(d))
}

// CurrentURL returns the last known URL for a session.
func (m *Manager) CurrentURL(sessionID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[sessionID]; ok {
		return s.url
	}
	return m.loadColdURL(sessionID)
}

// ─── Web Search (one-shot) ───────────────────────────────────────────────────

// WebSearch opens a search URL, waits for the page to load, extracts visible
// text, and returns the result. Does not register a persistent browser session.
func (m *Manager) WebSearch(ctx context.Context, url string, maxChars int) (*SearchResult, error) {
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), m.allocOpts()...)
	browserCtx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	defer allocCancel()

	// The context must allow the browser to run even after the outer ctx is done.
	// Use a detached context for navigation.
	navCtx, navCancel := context.WithTimeout(browserCtx, 30*time.Second)
	defer navCancel()

	var title string
	var bodyText string
	err := chromedp.Run(navCtx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var t string
			if err := chromedp.Evaluate("document.title", &t).Do(ctx); err == nil {
				title = t
			}
			return nil
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var text string
			if err := chromedp.Evaluate(fmt.Sprintf(`document.body.innerText.substring(0, %d)`, maxChars), &text).Do(ctx); err == nil {
				bodyText = text
			}
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("web search: %w", err)
	}

	snippet := bodyText
	if len(snippet) > 200 {
		snippet = snippet[:200] + "..."
	}
	return &SearchResult{
		URL:     url,
		Title:   title,
		Snippet: snippet,
		Content: bodyText,
	}, nil
}
