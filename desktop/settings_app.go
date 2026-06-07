package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"deepseek-anka/internal/agent"
	"deepseek-anka/internal/boot"
	"deepseek-anka/internal/config"
	"deepseek-anka/internal/provider"
	"deepseek-anka/internal/vision"
)

// settings_app.go is the desktop Settings panel's command surface: it reads the
// resolved config and applies edits through internal/config/edit.go (the
// purpose-built mutation API), then rebuilds the controller so the change takes
// effect live — the same snapshot→reload→resume pattern as SetModel. Secrets are
// the exception: they go to the global credentials file (upsertDotEnv), since
// config stores only the env-var name, not the key.

// --- read ---

type ProviderView struct {
	Name          string   `json:"name"`
	Kind          string   `json:"kind"`
	BaseURL       string   `json:"baseUrl"`
	Models        []string `json:"models"`
	Default       string   `json:"default"`
	APIKeyEnv     string   `json:"apiKeyEnv"`
	KeySet        bool     `json:"keySet"` // the env var currently resolves to a non-empty value
	BalanceURL    string   `json:"balanceUrl"`
	ContextWindow int      `json:"contextWindow"`
}

type PermissionsView struct {
	Mode  string   `json:"mode"`
	Allow []string `json:"allow"`
	Ask   []string `json:"ask"`
	Deny  []string `json:"deny"`
}

type SandboxView struct {
	Bash          string   `json:"bash"`
	Network       bool     `json:"network"`
	WorkspaceRoot string   `json:"workspaceRoot"`
	AllowWrite    []string `json:"allowWrite"`
}

type NetworkProxyView struct {
	Type     string `json:"type"`
	Server   string `json:"server"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type NetworkView struct {
	ProxyMode string           `json:"proxyMode"`
	ProxyURL  string           `json:"proxyUrl"`
	NoProxy   string           `json:"noProxy"`
	Proxy     NetworkProxyView `json:"proxy"`
}

type AgentView struct {
	Temperature  float64 `json:"temperature"`
	MaxSteps     int     `json:"maxSteps"`
	SystemPrompt string  `json:"systemPrompt"`
}

// VisionView holds auxiliary vision settings (separate from the chat model).
type VisionModelRefView struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
}

type VisionView struct {
	Enabled  bool                `json:"enabled"`
	Model    string              `json:"model"` // provider/model ref; required when enabled
	ModelRef *VisionModelRefView `json:"modelRef,omitempty"`
}

// ExtendedCapabilitiesView holds openhanako-ported capability toggles.
type ExtendedCapabilitiesView struct {
	ComputerEnabled            bool   `json:"computerEnabled"`
	AllowWindowsInputInjection bool   `json:"allowWindowsInputInjection"`
	BrowserEnabled             bool   `json:"browserEnabled"`
	BrowserHeadless            bool   `json:"browserHeadless"`
	BridgeEnabled              bool   `json:"bridgeEnabled"`
	BridgeAddr                 string `json:"bridgeAddr"`
	FeishuEnabled              bool   `json:"feishuEnabled"`
	FeishuAppID                string `json:"feishuAppId"`
	FeishuAppSecret            string `json:"feishuAppSecret"`
	WeChatEnabled              bool   `json:"wechatEnabled"`
	WeChatBotToken             string `json:"wechatBotToken"`
	QQEnabled                  bool   `json:"qqEnabled"`
	QQAppID                    string `json:"qqAppId"`
	QQAppSecret                string `json:"qqAppSecret"`
}

// SettingsView is the whole Settings panel payload.
type SettingsView struct {
	DefaultModel          string                   `json:"defaultModel"`
	PlannerModel          string                   `json:"plannerModel"`
	AutoPlan              string                   `json:"autoPlan"`
	Vision                VisionView               `json:"vision"`
	Capabilities          ExtendedCapabilitiesView `json:"capabilities"`
	Providers             []ProviderView           `json:"providers"`
	Permissions           PermissionsView          `json:"permissions"`
	Sandbox               SandboxView              `json:"sandbox"`
	Network               NetworkView              `json:"network"`
	Agent                 AgentView                `json:"agent"`
	DesktopLanguage       string                   `json:"desktopLanguage"`
	DesktopTheme          string                   `json:"desktopTheme"`
	DesktopThemeStyle     string                   `json:"desktopThemeStyle"`
	CloseBehavior         string                   `json:"closeBehavior"`
	ConfigPath            string                   `json:"configPath"`
	VisionModelCandidates []string                 `json:"visionModelCandidates"`
	// ProviderKinds lists the provider implementations the kernel actually
	// registered (provider.Kinds()), so the editor's "kind" picker offers only
	// kinds that resolve — selecting an unregistered one would fail the rebuild.
	ProviderKinds []string `json:"providerKinds"`
	// Bypass is the live YOLO state (runtime-only, not from config), so the panel's
	// toggle reflects whether approvals are currently being skipped this session.
	Bypass bool `json:"bypass"`
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// Settings returns the current configuration for the Settings panel.
func (a *App) Settings() SettingsView {
	cfg, cfgPath, err := a.loadDesktopUserConfigForEdit()
	if err != nil {
		return SettingsView{
			Providers:     []ProviderView{},
			ProviderKinds: nonNil(provider.Kinds()),
			Permissions: PermissionsView{
				Mode:  "ask",
				Allow: []string{},
				Ask:   []string{},
				Deny:  []string{},
			},
			Sandbox:           SandboxView{Bash: "enforce", AllowWrite: []string{}},
			AutoPlan:          "off",
			DesktopTheme:      "dark",
			DesktopThemeStyle: "graphite",
			CloseBehavior:     effectiveDesktopCloseBehavior("background"),
		}
	}
	ctrl := a.activeCtrl()
	bash := cfg.Sandbox.Bash
	if bash == "" {
		bash = "enforce"
	}
	v := SettingsView{
		DefaultModel: cfg.DefaultModel,
		PlannerModel: cfg.Agent.PlannerModel,
		AutoPlan:     desktopAutoPlanMode(cfg.Agent.AutoPlan),
		Providers:    []ProviderView{},
		Permissions: PermissionsView{
			Mode:  orDefault(cfg.Permissions.Mode, "ask"),
			Allow: nonNil(cfg.Permissions.Allow),
			Ask:   nonNil(cfg.Permissions.Ask),
			Deny:  nonNil(cfg.Permissions.Deny),
		},
		Sandbox: SandboxView{
			Bash: bash, Network: cfg.Sandbox.Network,
			WorkspaceRoot: cfg.Sandbox.WorkspaceRoot, AllowWrite: nonNil(cfg.Sandbox.AllowWrite),
		},
		Network: NetworkView{
			ProxyMode: cfg.NetworkProxyMode(),
			ProxyURL:  cfg.Network.ProxyURL,
			NoProxy:   cfg.Network.NoProxy,
			Proxy: NetworkProxyView{
				Type:     orDefault(cfg.Network.Proxy.Type, "socks5"),
				Server:   cfg.Network.Proxy.Server,
				Port:     cfg.Network.Proxy.Port,
				Username: cfg.Network.Proxy.Username,
				Password: cfg.Network.Proxy.Password,
			},
		},
		Agent:  AgentView{Temperature: cfg.Agent.Temperature, MaxSteps: cfg.Agent.MaxSteps, SystemPrompt: cfg.Agent.SystemPrompt},
		Vision: visionViewFromConfig(cfg),
		Capabilities: ExtendedCapabilitiesView{
			ComputerEnabled:            cfg.Computer.Enabled,
			AllowWindowsInputInjection: cfg.Computer.AllowWindowsInputInjection,
			BrowserEnabled:             cfg.Browser.Enabled,
			BrowserHeadless:            cfg.Browser.Headless,
			BridgeEnabled:              cfg.Bridge.Enabled,
			BridgeAddr:                 cfg.Bridge.Addr,
			FeishuEnabled:              cfg.Bridge.Feishu.Enabled,
			FeishuAppID:                cfg.Bridge.Feishu.AppID,
			FeishuAppSecret:            cfg.Bridge.Feishu.AppSecret,
			WeChatEnabled:              cfg.Bridge.WeChat.Enabled,
			WeChatBotToken:             cfg.Bridge.WeChat.BotToken,
			QQEnabled:                  cfg.Bridge.QQ.Enabled,
			QQAppID:                    cfg.Bridge.QQ.AppID,
			QQAppSecret:                cfg.Bridge.QQ.AppSecret,
		},
		VisionModelCandidates: visionModelCandidates(cfg),
		DesktopLanguage:       cfg.DesktopLanguage(),
		DesktopTheme:          cfg.DesktopTheme(),
		DesktopThemeStyle:     cfg.DesktopThemeStyle(),
		CloseBehavior:         effectiveDesktopCloseBehavior(cfg.DesktopCloseBehavior()),
		ConfigPath:            cfgPath,
		ProviderKinds:         nonNil(provider.Kinds()),
		Bypass:                ctrl != nil && ctrl.Bypass(),
	}
	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		v.Providers = append(v.Providers, ProviderView{
			Name: p.Name, Kind: p.Kind, BaseURL: p.BaseURL,
			Models: nonNil(p.ModelList()), Default: p.DefaultModel(),
			APIKeyEnv:     p.APIKeyEnv,
			KeySet:        p.APIKeyEnv != "" && os.Getenv(p.APIKeyEnv) != "",
			BalanceURL:    p.BalanceURL,
			ContextWindow: p.ContextWindow,
		})
	}
	return v
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

// --- apply (write config, then rebuild the controller so it's live) ---

// applyConfigChange mutates the user-global config and rebuilds the controller so
// the change takes effect this session. Desktop settings such as providers and
// keys are account-level, not per-project: writing them to the global config
// rather than the cwd's reasonix.toml is what lets them survive a workspace switch.
func (a *App) applyConfigChange(mutate func(*config.Config) error) error {
	cfg, path, err := a.loadDesktopUserConfigForEdit()
	if err != nil {
		return err
	}
	if err := mutate(cfg); err != nil {
		return err
	}
	if err := cfg.SaveTo(path); err != nil {
		return err
	}
	return a.rebuild()
}

func (a *App) applyConfigOnly(mutate func(*config.Config) error) error {
	cfg, path, err := a.loadDesktopUserConfigForEdit()
	if err != nil {
		return err
	}
	if err := mutate(cfg); err != nil {
		return err
	}
	return cfg.SaveTo(path)
}

func (a *App) loadDesktopUserConfigForEdit() (*config.Config, string, error) {
	userPath := config.UserConfigPath()
	if userPath == "" {
		return nil, "", fmt.Errorf("cannot resolve user config directory")
	}
	var cfg *config.Config
	if _, err := os.Stat(userPath); err == nil {
		cfg = config.LoadForEdit(userPath)
	} else {
		cfg = config.LoadForEdit(userPath)
		legacyPath := config.SourcePathForRoot(a.activeWorkspaceRoot())
		if legacyPath != "" && !sameConfigPath(legacyPath, userPath) {
			cfg = config.LoadForEdit(legacyPath)
			cfg.ConfigVersion = config.Default().ConfigVersion
		}
	}
	if repairCorruptedProviderKeys(cfg) {
		_ = cfg.SaveTo(userPath)
	}
	return cfg, userPath, nil
}

func (a *App) activeWorkspaceRoot() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if tab := a.activeTabLocked(); tab != nil {
		return tab.WorkspaceRoot
	}
	return "."
}

func projectConfigPathForRoot(root string) string {
	if strings.TrimSpace(root) == "" || root == "." {
		return "reasonix.toml"
	}
	return filepath.Join(root, "reasonix.toml")
}

func sameConfigPath(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	aAbs, aErr := filepath.Abs(a)
	bAbs, bErr := filepath.Abs(b)
	if aErr == nil && bErr == nil {
		return filepath.Clean(aAbs) == filepath.Clean(bAbs)
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// rebuild tears down the controller and rebuilds it from the (just-changed)
// config, carrying the conversation forward. It keeps the active model if it
// still resolves; otherwise it falls back to the new default. Mirrors SetModel.
func (a *App) rebuild() error {
	if a.ctx == nil {
		return nil
	}
	tab := a.activeTab()
	if tab == nil {
		return fmt.Errorf("no active tab")
	}
	var carried []provider.Message
	prevPath := ""
	if tab.Ctrl != nil {
		prevPath = tab.Ctrl.SessionPath()
		_ = tab.Ctrl.Snapshot()
		carried = tab.Ctrl.History()
		tab.Ctrl.Close()
	}
	model := tab.model
	if cfg, err := config.LoadForRoot(tab.WorkspaceRoot); err == nil {
		if _, ok := cfg.ResolveModel(model); !ok {
			model = cfg.DefaultModel
			if e, ok := cfg.ResolveModel(model); ok {
				model = e.Name + "/" + e.Model
			}
		}
	}
	ctrl, err := boot.Build(a.bootContext(), boot.Options{
		Model: model, RequireKey: false,
		Sink:           tab.sink,
		WorkspaceRoot:  tab.WorkspaceRoot,
		EffortOverride: cloneStringPtr(tab.effort),
	})
	if err != nil {
		a.mu.Lock()
		tab.StartupErr = err.Error()
		tab.Ready = true
		a.mu.Unlock()
		a.emitReady(a.ctx)
		return err
	}
	a.mu.Lock()
	tab.Ctrl = ctrl
	tab.model = model
	tab.Label = ctrl.Label()
	tab.StartupErr = ""
	tab.Ready = true
	a.saveTabsLocked()
	a.mu.Unlock()
	a.emitReady(a.ctx)
	ctrl.EnableInteractiveApproval()
	applyTabModeToController(ctrl, tab.mode)
	path := agent.ContinueSessionPath(prevPath, ctrl.SessionDir(), ctrl.Label())
	if len(carried) > 0 {
		carried = withFreshSystemPrompt(carried, systemPromptFrom(ctrl.History()))
		ctrl.Resume(&agent.Session{Messages: carried}, path)
	} else if path != "" {
		ctrl.SetSessionPath(path)
	}
	return nil
}

func systemPromptFrom(messages []provider.Message) string {
	for _, m := range messages {
		if m.Role == provider.RoleSystem {
			return m.Content
		}
	}
	return ""
}

func withFreshSystemPrompt(messages []provider.Message, system string) []provider.Message {
	if strings.TrimSpace(system) == "" {
		return messages
	}
	out := append([]provider.Message(nil), messages...)
	for i := range out {
		if out[i].Role == provider.RoleSystem {
			out[i].Content = system
			out[i].ReasoningContent = ""
			out[i].ReasoningSignature = ""
			out[i].ToolCalls = nil
			out[i].ToolCallID = ""
			out[i].Name = ""
			return out
		}
	}
	return append([]provider.Message{{Role: provider.RoleSystem, Content: system}}, out...)
}

// SetDefaultModel sets the config default and switches the live model to it.
func (a *App) SetDefaultModel(ref string) error {
	tab := a.activeTab()
	if tab == nil {
		return fmt.Errorf("no active tab")
	}
	prev := tab.model
	tab.model = ref
	if err := a.applyConfigChange(func(c *config.Config) error {
		if _, ok := c.ResolveModel(ref); !ok {
			return fmt.Errorf("unknown model %q", ref)
		}
		c.DefaultModel = ref
		return nil
	}); err != nil {
		tab.model = prev
		return err
	}
	return nil
}

// SetPlannerModel sets (or, with "", clears) the two-model planner.
func (a *App) SetPlannerModel(ref string) error {
	return a.applyConfigChange(func(c *config.Config) error {
		if ref != "" {
			if _, ok := c.ResolveModel(ref); !ok {
				return fmt.Errorf("unknown planner model %q", ref)
			}
		}
		c.Agent.PlannerModel = ref
		return nil
	})
}

// SetAutoPlan updates the automatic plan-mode gate (off|on).
func (a *App) SetAutoPlan(mode string) error {
	return a.applyConfigChange(func(c *config.Config) error { return c.SetAutoPlan(mode) })
}

func desktopAutoPlanMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "on", "ask":
		return "on"
	default:
		return "off"
	}
}

// SaveProvider adds or updates a provider. A single model fills `model`; several
// fill `models` (with `default`). The shared key/endpoint live on the entry.
func (a *App) SaveProvider(p ProviderView) error {
	if err := validateProviderAPIKeyEnv(p.APIKeyEnv); err != nil {
		return err
	}
	return a.applyConfigChange(func(c *config.Config) error {
		e := config.ProviderEntry{
			Name: p.Name, Kind: p.Kind, BaseURL: p.BaseURL,
			APIKeyEnv: p.APIKeyEnv, BalanceURL: strings.TrimSpace(p.BalanceURL), ContextWindow: p.ContextWindow,
		}
		if len(p.Models) > 0 {
			e.Model = p.Models[0] // also satisfies validateProvider's model requirement
			if len(p.Models) > 1 {
				e.Models = p.Models
				e.Default = p.Default
			}
		}
		return c.UpsertProvider(e)
	})
}

// DeleteProvider removes a provider (refused for the current default_model).
func (a *App) DeleteProvider(name string) error {
	return a.applyConfigChange(func(c *config.Config) error { return c.RemoveProvider(name) })
}

// SetProviderKey writes a secret to the global credentials file under the given
// env-var name (the one a provider's api_key_env points at) and rebuilds so it
// resolves immediately.
func (a *App) SetProviderKey(apiKeyEnv, value string) error {
	if strings.TrimSpace(apiKeyEnv) == "" {
		return fmt.Errorf("this provider has no api_key_env set")
	}
	if err := upsertDotEnv(apiKeyEnv, value); err != nil {
		return err
	}
	return a.rebuild()
}

// SetPermissionMode sets the writer-fallback mode (ask|allow|deny).
func (a *App) SetPermissionMode(mode string) error {
	return a.applyConfigChange(func(c *config.Config) error { return c.SetPermissionMode(mode) })
}

// AddPermissionRule appends a rule to the allow/ask/deny list.
func (a *App) AddPermissionRule(list, rule string) error {
	return a.applyConfigChange(func(c *config.Config) error { return c.AddPermissionRule(list, rule) })
}

// RemovePermissionRule drops a rule from the allow/ask/deny list.
func (a *App) RemovePermissionRule(list, rule string) error {
	return a.applyConfigChange(func(c *config.Config) error {
		_, err := c.RemovePermissionRule(list, rule)
		return err
	})
}

// SetSandbox updates the bash sandbox mode, network egress, and write roots.
func (a *App) SetSandbox(bash string, network bool, workspaceRoot string, allowWrite []string) error {
	return a.applyConfigChange(func(c *config.Config) error {
		c.Sandbox.Bash = bash
		c.Sandbox.Network = network
		c.Sandbox.WorkspaceRoot = strings.TrimSpace(workspaceRoot)
		c.Sandbox.AllowWrite = trimList(allowWrite)
		return nil
	})
}

// SetNetwork updates ordinary outbound proxy settings.
func (a *App) SetNetwork(n NetworkView) error {
	return a.applyConfigChange(func(c *config.Config) error {
		return c.SetNetwork(config.NetworkConfig{
			ProxyMode: n.ProxyMode,
			ProxyURL:  n.ProxyURL,
			NoProxy:   n.NoProxy,
			Proxy: config.NetworkProxyConfig{
				Type:     n.Proxy.Type,
				Server:   n.Proxy.Server,
				Port:     n.Proxy.Port,
				Username: n.Proxy.Username,
				Password: n.Proxy.Password,
			},
		})
	})
}

// SetCloseBehavior updates desktop-only window close behavior without rebuilding
// the active controller. It must stay out of provider-visible prompt/request data.
func (a *App) SetCloseBehavior(mode string) error {
	if mode == "background" && !shouldHideWindowOnClose(mode) {
		mode = "quit"
	}
	return a.applyConfigOnly(func(c *config.Config) error { return c.SetDesktopCloseBehavior(mode) })
}

// SetDesktopLanguage updates only the desktop UI language. It deliberately does
// not touch config.language, which the CLI/model-facing runtime uses.
func (a *App) SetDesktopLanguage(lang string) error {
	return a.applyConfigOnly(func(c *config.Config) error { return c.SetDesktopLanguage(lang) })
}

// SetDesktopAppearance updates only desktop theme preferences. It does not
// rebuild the active controller and must stay out of provider-visible requests.
func (a *App) SetDesktopAppearance(theme, style string) error {
	return a.applyConfigOnly(func(c *config.Config) error { return c.SetDesktopAppearance(theme, style) })
}

// MigrateDesktopPreferences imports old browser-local desktop preferences into
// the user config once. Existing [desktop] values win so stale localStorage never
// overwrites an explicit config edit.
func (a *App) MigrateDesktopPreferences(language, theme, style string) error {
	return a.applyConfigOnly(func(c *config.Config) error {
		if strings.TrimSpace(c.Desktop.Language) == "" {
			if err := c.SetDesktopLanguage(language); err != nil {
				return err
			}
		}
		if strings.TrimSpace(c.Desktop.Theme) == "" && strings.TrimSpace(c.Desktop.ThemeStyle) == "" {
			if err := c.SetDesktopAppearance(theme, style); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetVision configures auxiliary vision (requires a separate vision-capable model).
// Toggling enabled alone does not validate a previously stored model — validation
// runs only when model is explicitly set in the same call.
func (a *App) SetVision(enabled bool, model string) error {
	ref := strings.TrimSpace(model)
	mutate := func(c *config.Config) error {
		c.Vision.Enabled = enabled
		if !enabled {
			return nil
		}
		if ref == "" {
			return nil
		}
		c.Vision.Model = ref
		rc, err := vision.ResolveVisionConfig(c)
		if err != nil {
			return err
		}
		if rc == nil || rc.Entry == nil {
			return fmt.Errorf("vision model %q not found in [[providers]]. Add a vision-capable provider first (e.g. GPT-4o).", ref)
		}
		if !vision.ModelSupportsImage(rc.Entry, rc.ModelID) {
			return fmt.Errorf("vision model must support image input. Select a vision-capable model such as gpt-4o or qwen-vl-max")
		}
		return nil
	}
	// Enable/disable without choosing a model should not block on controller rebuild —
	// users often turn vision on first, then add a vision provider and pick a model.
	if ref == "" {
		if err := a.applyConfigOnly(mutate); err != nil {
			return err
		}
		if !enabled {
			return a.rebuild()
		}
		return nil
	}
	return a.applyConfigChange(mutate)
}

// SetupVisionProvider adds or updates a vision-capable provider, stores its API key,
// and selects it as the auxiliary vision model in one step.
func (a *App) SetupVisionProvider(p ProviderView, visionModelRef string, apiKey string) error {
	ref := strings.TrimSpace(visionModelRef)
	if ref == "" {
		return fmt.Errorf("vision model ref is required")
	}
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.BaseURL) == "" {
		return fmt.Errorf("provider name and base_url are required")
	}
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return fmt.Errorf("API key is required to enable vision")
	}
	if strings.TrimSpace(p.APIKeyEnv) == "" {
		return fmt.Errorf("provider api_key_env is required")
	}
	if err := validateProviderAPIKeyEnv(p.APIKeyEnv); err != nil {
		return err
	}
	if err := upsertDotEnv(p.APIKeyEnv, key); err != nil {
		return err
	}
	return a.applyConfigChange(func(c *config.Config) error {
		e := config.ProviderEntry{
			Name: p.Name, Kind: p.Kind, BaseURL: p.BaseURL,
			APIKeyEnv: p.APIKeyEnv, BalanceURL: strings.TrimSpace(p.BalanceURL), ContextWindow: p.ContextWindow,
		}
		if len(p.Models) > 0 {
			e.Model = p.Models[0]
			if len(p.Models) > 1 {
				e.Models = p.Models
				e.Default = p.Default
			}
		}
		if err := c.UpsertProvider(e); err != nil {
			return err
		}
		c.Vision.Enabled = true
		c.Vision.Model = ref
		slash := strings.Index(ref, "/")
		if slash <= 0 {
			return fmt.Errorf("vision model ref must be provider/model")
		}
		provName := ref[:slash]
		modelID := ref[slash+1:]
		entry, ok := c.ResolveModel(provName)
		if !ok || entry == nil {
			return fmt.Errorf("vision provider %q not found after save", provName)
		}
		if !vision.ModelSupportsImage(entry, modelID) {
			return fmt.Errorf("vision model must support image input (e.g. gpt-4o, qwen-vl-max)")
		}
		if _, err := c.ResolveVisionModel(); err != nil {
			return err
		}
		return nil
	})
}

func visionModelCandidates(cfg *config.Config) []string {
	if cfg == nil {
		return []string{}
	}
	out := []string{}
	seen := map[string]bool{}
	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		for _, m := range p.ModelList() {
			if !vision.ModelSupportsImage(p, m) {
				continue
			}
			ref := p.Name + "/" + m
			if seen[ref] {
				continue
			}
			seen[ref] = true
			out = append(out, ref)
		}
	}
	return out
}

func visionViewFromConfig(cfg *config.Config) VisionView {
	v := VisionView{Enabled: cfg.Vision.Enabled, Model: cfg.Vision.Model}
	if ref, ok := cfg.ResolveModel(cfg.Vision.Model); ok && ref != nil {
		id := ref.Model
		if id == "" {
			id = ref.DefaultModel()
		}
		v.ModelRef = &VisionModelRefView{ID: id, Provider: ref.Name}
	}
	return v
}

// SetExtendedCapabilities updates computer/browser/bridge toggles.
func (a *App) SetExtendedCapabilities(cap ExtendedCapabilitiesView) error {
	err := a.applyConfigChange(func(c *config.Config) error {
		c.Computer.Enabled = cap.ComputerEnabled
		c.Computer.AllowWindowsInputInjection = cap.AllowWindowsInputInjection
		c.Browser.Enabled = cap.BrowserEnabled
		c.Browser.Headless = cap.BrowserHeadless
		c.Bridge.Enabled = cap.BridgeEnabled
		if strings.TrimSpace(cap.BridgeAddr) != "" {
			c.Bridge.Addr = strings.TrimSpace(cap.BridgeAddr)
		}
		if c.Bridge.Addr == "" {
			c.Bridge.Addr = "127.0.0.1:8787"
		}
		c.Bridge.Feishu.Enabled = cap.FeishuEnabled
		c.Bridge.Feishu.AppID = strings.TrimSpace(cap.FeishuAppID)
		c.Bridge.Feishu.AppSecret = strings.TrimSpace(cap.FeishuAppSecret)
		c.Bridge.WeChat.Enabled = cap.WeChatEnabled
		c.Bridge.WeChat.BotToken = strings.TrimSpace(cap.WeChatBotToken)
		c.Bridge.QQ.Enabled = cap.QQEnabled
		c.Bridge.QQ.AppID = strings.TrimSpace(cap.QQAppID)
		c.Bridge.QQ.AppSecret = strings.TrimSpace(cap.QQAppSecret)
		if cap.FeishuEnabled || cap.WeChatEnabled || cap.QQEnabled {
			c.Bridge.Enabled = true
		}
		return nil
	})
	if err != nil {
		return err
	}
	a.syncBridgeSidecar()
	return nil
}

// SetAgentParams updates sampling temperature, the optional max-steps guard, and
// the base system prompt.
func (a *App) SetAgentParams(temperature float64, maxSteps int, systemPrompt string) error {
	return a.applyConfigChange(func(c *config.Config) error {
		c.Agent.Temperature = temperature
		c.Agent.MaxSteps = maxSteps
		c.Agent.SystemPrompt = systemPrompt
		return nil
	})
}

// trimList drops blank entries from a string slice (and returns a non-nil slice).
func trimList(in []string) []string {
	out := []string{}
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func validateProviderAPIKeyEnv(env string) error {
	env = strings.TrimSpace(env)
	if env == "" {
		return fmt.Errorf("provider api_key_env is required")
	}
	if config.APIKeyEnvLooksLikeSecret(env) {
		return fmt.Errorf("api_key_env must be an environment variable name (e.g. MOONSHOT_API_KEY), not the API key itself")
	}
	if !config.ValidAPIKeyEnvName(env) {
		return fmt.Errorf("api_key_env %q is not a valid environment variable name", env)
	}
	return nil
}

// repairCorruptedProviderKeys moves secrets mistakenly stored in api_key_env into
// the credentials file under the proper env var name.
func repairCorruptedProviderKeys(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	changed := false
	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		if !config.APIKeyEnvLooksLikeSecret(p.APIKeyEnv) {
			continue
		}
		secret := strings.TrimSpace(p.APIKeyEnv)
		env := config.DefaultAPIKeyEnvForProvider(p.Name)
		if err := upsertDotEnv(env, secret); err != nil {
			continue
		}
		p.APIKeyEnv = env
		changed = true
	}
	return changed
}

// repairBrokenVisionRef keeps old/bad preset refs from leaving vision enabled but
// pointed at a provider that is no longer configured. Prefer the same model under
// another provider; otherwise choose the first configured image-capable model.
func repairBrokenVisionRef(cfg *config.Config) bool {
	if cfg == nil || !cfg.Vision.Enabled || strings.TrimSpace(cfg.Vision.Model) == "" {
		return false
	}
	if _, ok := cfg.ResolveModel(cfg.Vision.Model); ok {
		return false
	}
	_, wantedModel, hasProvider := strings.Cut(strings.TrimSpace(cfg.Vision.Model), "/")
	if hasProvider {
		for i := range cfg.Providers {
			p := &cfg.Providers[i]
			if p.HasModel(wantedModel) && vision.ModelSupportsImage(p, wantedModel) {
				cfg.Vision.Model = p.Name + "/" + wantedModel
				return true
			}
		}
	}
	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		for _, model := range p.ModelList() {
			if vision.ModelSupportsImage(p, model) {
				cfg.Vision.Model = p.Name + "/" + model
				return true
			}
		}
	}
	return false
}

func repairUserConfigOnStartup() {
	cleanInvalidCredentialKeys()
	path := config.UserConfigPath()
	if path == "" {
		return
	}
	cfg := config.LoadForEdit(path)
	changed := repairCorruptedProviderKeys(cfg)
	if repairBrokenVisionRef(cfg) {
		changed = true
	}
	if !changed {
		return
	}
	_ = cfg.SaveTo(path)
}
