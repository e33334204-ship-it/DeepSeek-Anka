package vision

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"reasonix/internal/config"
)

// ResolveVisionConfigFunc resolves auxiliary vision model credentials.
type ResolveVisionConfigFunc func() (*ResolvedConfig, error)

// Bridge implements openhanako VisionBridge for Reasonix.
type Bridge struct {
	resolve ResolveVisionConfigFunc
	client  *visionClient
	now     func() int64

	mu              sync.Mutex
	analysisByPrompt map[string]cacheEntry
	noteByPath       map[string]*NoteEntry
	maxCacheEntries  int
	visionMaxTokens  int
}

type cacheEntry struct {
	Note       string
	CreatedAt  int64
	LastUsedAt int64
	Index      int
}

// NewBridge builds a vision bridge. Returns nil when vision is disabled.
func NewBridge(cfg *config.Config) (*Bridge, error) {
	if cfg == nil || !cfg.Vision.Enabled {
		return nil, nil
	}
	if _, err := cfg.ResolveVisionModel(); err != nil {
		return nil, err
	}
	proxy := cfg.NetworkProxySpec()
	client, err := newVisionClient(proxy)
	if err != nil {
		return nil, err
	}
	resolve := func() (*ResolvedConfig, error) {
		return ResolveVisionConfig(cfg)
	}
	return &Bridge{
		resolve:          resolve,
		client:           client,
		now:              func() int64 { return time.Now().UnixMilli() },
		analysisByPrompt: map[string]cacheEntry{},
		noteByPath:       map[string]*NoteEntry{},
		maxCacheEntries:  maxCacheEntries,
		visionMaxTokens:  defaultVisionMaxTokens,
	}, nil
}

// ModelRef returns display ref for the configured vision model.
func (b *Bridge) ModelRef() string {
	if b == nil {
		return ""
	}
	cfg, err := b.resolve()
	if err != nil || cfg == nil {
		return ""
	}
	return cfg.Provider + "/" + cfg.ModelID
}

// Prepare analyzes images for a text-only target model (vision-bridge.prepare).
func (b *Bridge) Prepare(ctx context.Context, opts PrepareOptions) (*PrepareResult, error) {
	if b == nil {
		return nil, fmt.Errorf("vision bridge is not configured")
	}
	if len(opts.Images) == 0 {
		return &PrepareResult{Text: opts.Text, Images: opts.Images}, nil
	}
	if !RequiresAuxiliaryVision(opts.TargetModel) {
		return &PrepareResult{Text: opts.Text, Images: opts.Images}, nil
	}

	cfg, err := b.resolve()
	if err != nil {
		return nil, err
	}
	if cfg == nil || cfg.Entry == nil {
		return nil, errors.New("vision auxiliary model is required for image input with the current text-only model")
	}
	if !ModelSupportsImage(cfg.Entry, cfg.ModelID) {
		return nil, errors.New("vision auxiliary model must support image input")
	}

	paths := opts.ImageAttachmentPaths
	if len(paths) == 0 {
		paths = UniqueImagePathsFromText(opts.Text)
	}
	userRequest := normalizeUserRequest(opts.Text)
	var notes []string
	for i, img := range opts.Images {
		note, err := b.analyzeImage(ctx, cfg, img, i, userRequest, opts.SessionPath)
		if err != nil {
			return nil, err
		}
		imagePath := ""
		if i < len(paths) {
			imagePath = paths[i]
		}
		if imagePath != "" {
			entry := &NoteEntry{
				Note:        note,
				SessionPath: opts.SessionPath,
				ImagePath:   imagePath,
				UserRequest: userRequest,
				VisionModel: compactModelRef(cfg.Entry),
				TargetModel: &opts.TargetModel.Ref,
				UpdatedAt:   b.now(),
			}
			b.storeNote(imagePath, entry, opts.SessionPath)
		}
		notes = append(notes, note)
	}
	return &PrepareResult{Text: opts.Text, VisionNotes: notes}, nil
}

// PrepareResources summarizes keyed image resources (vision-bridge.prepareResources).
func (b *Bridge) PrepareResources(ctx context.Context, opts ResourcesOptions) ([]PreparedNote, error) {
	if b == nil {
		return nil, fmt.Errorf("vision bridge is not configured")
	}
	if len(opts.Resources) == 0 {
		return nil, nil
	}
	if opts.TargetModel.Entry != nil && !RequiresAuxiliaryVision(opts.TargetModel) {
		return nil, nil
	}
	return b.summarizeResources(ctx, opts)
}

// SummarizeResources runs vision analysis even without a text-only target model.
func (b *Bridge) SummarizeResources(ctx context.Context, opts ResourcesOptions) ([]PreparedNote, error) {
	if b == nil {
		return nil, fmt.Errorf("vision bridge is not configured")
	}
	if len(opts.Resources) == 0 {
		return nil, nil
	}
	opts.TargetModel = TargetModel{}
	return b.summarizeResources(ctx, opts)
}

func (b *Bridge) summarizeResources(ctx context.Context, opts ResourcesOptions) ([]PreparedNote, error) {
	cfg, err := b.resolve()
	if err != nil {
		return nil, err
	}
	if cfg == nil || cfg.Entry == nil {
		return nil, errors.New("vision auxiliary model is required for image input with the current text-only model")
	}
	if !ModelSupportsImage(cfg.Entry, cfg.ModelID) {
		return nil, errors.New("vision auxiliary model must support image input")
	}

	request := normalizeUserRequest(opts.UserRequest)
	if request == "" {
		request = normalizeUserRequest(opts.Text)
	}
	var notes []PreparedNote
	for i, resource := range opts.Resources {
		key := strings.TrimSpace(resource.Key)
		if key == "" || len(resource.Image.Data) == 0 {
			continue
		}
		if existing := b.lookupNote(opts.SessionPath, key); existing != nil && existing.Note != "" {
			notes = append(notes, PreparedNote{
				Key: key, Label: resource.Label, Note: existing.Note, Reused: true,
			})
			continue
		}
		note, err := b.analyzeImage(ctx, cfg, resource.Image, i, request, opts.SessionPath)
		if err != nil {
			return nil, err
		}
		entry := &NoteEntry{
			Note:        note,
			SessionPath: opts.SessionPath,
			ImagePath:   key,
			UserRequest: request,
			VisionModel: compactModelRef(cfg.Entry),
			TargetModel: &opts.TargetModel.Ref,
			UpdatedAt:   b.now(),
		}
		b.storeNote(key, entry, opts.SessionPath)
		label := resource.Label
		if label == "" {
			label = key
		}
		notes = append(notes, PreparedNote{Key: key, Label: label, Note: note})
	}
	return notes, nil
}

// LookupNote returns a cached note for an image path.
func (b *Bridge) LookupNote(sessionPath, imagePath string) *NoteEntry {
	if b == nil {
		return nil
	}
	return b.lookupNote(sessionPath, imagePath)
}

// InjectNotes prepends vision-context blocks into user message strings.
func (b *Bridge) InjectNotes(messages []string, sessionPath string) ([]string, int) {
	if b == nil || len(messages) == 0 {
		return messages, 0
	}
	injected := 0
	out := make([]string, len(messages))
	for i, text := range messages {
		if text == "" || strings.Contains(text, ContextStart) {
			out[i] = text
			continue
		}
		paths := UniqueImagePathsFromText(text)
		if len(paths) == 0 {
			out[i] = text
			continue
		}
		var noteLines []string
		localHits := 0
		for idx, path := range paths {
			entry := b.lookupNote(sessionPath, path)
			if entry == nil || entry.Note == "" {
				continue
			}
			noteLines = append(noteLines, fmt.Sprintf("image_%d: %s", idx+1, entry.Note))
			localHits++
		}
		if localHits == 0 {
			out[i] = text
			continue
		}
		block := ContextStart + "\n" + strings.Join(noteLines, "\n\n") + "\n" + ContextEnd + "\n\n"
		out[i] = block + text
		injected += localHits
	}
	return out, injected
}

// Analyze is a convenience wrapper for single-image analysis (tools/refs).
func (b *Bridge) Analyze(ctx context.Context, imagePath, userRequest string) (string, error) {
	resource, err := LoadImageResource(imagePath, imagePath)
	if err != nil {
		return "", err
	}
	notes, err := b.SummarizeResources(ctx, ResourcesOptions{
		SessionPath: "",
		UserRequest: userRequest,
		Resources:   []Resource{resource},
	})
	if err != nil {
		return "", err
	}
	if len(notes) == 0 {
		return "", fmt.Errorf("vision analysis produced no note")
	}
	return notes[0].Note, nil
}

type PrepareOptions struct {
	SessionPath          string
	TargetModel          TargetModel
	Text                 string
	Images               []ImageInput
	ImageAttachmentPaths []string
}

type PrepareResult struct {
	Text        string
	Images      []ImageInput
	VisionNotes []string
}

type ResourcesOptions struct {
	SessionPath string
	TargetModel TargetModel
	UserRequest string
	Text        string
	Resources   []Resource
}

func (b *Bridge) analyzeImage(ctx context.Context, cfg *ResolvedConfig, img ImageInput, index int, userRequest, sessionPath string) (string, error) {
	normalized, err := NormalizeModelImageInput(img, index)
	if err != nil {
		return "", err
	}
	caps := GetVisionCapabilities(cfg.Entry, cfg.ModelID)
	key := imagePromptCacheKey(normalized, userRequest, visionModelCacheSignature(cfg, caps))
	b.mu.Lock()
	if cached, ok := b.analysisByPrompt[key]; ok {
		cached.LastUsedAt = b.now()
		b.analysisByPrompt[key] = cached
		b.mu.Unlock()
		return cached.Note, nil
	}
	b.mu.Unlock()

	var note string
	if caps != nil {
		note, err = b.analyzeImageWithPrimitives(ctx, cfg, normalized, userRequest, caps)
	} else {
		note, err = b.analyzeImageAsNote(ctx, cfg, normalized, userRequest)
	}
	if err != nil {
		return "", err
	}
	b.mu.Lock()
	b.analysisByPrompt[key] = cacheEntry{
		Note: note, CreatedAt: b.now(), LastUsedAt: b.now(), Index: index,
	}
	b.trimAnalysisCache()
	b.mu.Unlock()
	return note, nil
}

func (b *Bridge) analyzeImageAsNote(ctx context.Context, cfg *ResolvedConfig, img ImageInput, userRequest string) (string, error) {
	prompt := strings.Join([]string{
		"Analyze this image for another text-only model.",
		"Return a concise paper note with these exact sections:",
		"image_overview: fixed basic description of what the image is.",
		"visible_text: important OCR or readable text.",
		"objects_and_layout: important objects, positions, counts, and relationships.",
		"charts_or_data: chart/table/data details if present; otherwise say none.",
		"user_request: restate the user's request in one short sentence.",
		"user_request_answer: answer the user's request using the image when possible.",
		"evidence: the visual evidence supporting that answer.",
		"uncertainty: anything unclear, hidden, or guessed.",
		"Do not mention that you are a tool or a separate model.",
		"",
		"User request:\n" + orDefault(userRequest, "(no explicit text request)"),
	}, "\n")
	text, err := b.client.callText(ctx, cfg, prompt, img, b.maxTokensForModel(cfg))
	if err != nil {
		return "", err
	}
	return truncateText(text, maxNoteChars), nil
}

func (b *Bridge) analyzeImageWithPrimitives(ctx context.Context, cfg *ResolvedConfig, img ImageInput, userRequest string, caps *VisionCapabilities) (string, error) {
	shape := primitivePromptShape(caps)
	lines := []string{
		"Analyze this image for another text-only model.",
		"Return only one valid JSON object. Do not wrap it in Markdown.",
		"Use this exact shape:",
		"{",
		`  "image_overview": "fixed basic description of what the image is",`,
		`  "visible_text": ["important OCR or readable text"],`,
		`  "objects_and_layout": "important objects, positions, counts, and relationships",`,
		`  "charts_or_data": "chart/table/data details if present; otherwise none",`,
		`  "user_request": "restate the user request in one short sentence",`,
		`  "user_request_answer": "answer the user request using the image when possible",`,
		`  "evidence": "visual evidence supporting that answer",`,
		`  "uncertainty": "anything unclear, hidden, or guessed",`,
	}
	lines = append(lines, shape...)
	if caps.Points {
		lines = append(lines, "You may include point or center coordinates as [x, y] normalized to 0-1000.")
	} else {
		lines = append(lines, "Do not output point primitives.")
	}
	lines = append(lines,
		"Include only coordinates that matter for the user request or key spatial evidence.",
		"Do not mention that you are a tool or a separate model.",
		"",
		"User request:\n"+orDefault(userRequest, "(no explicit text request)"),
	)
	responseText, err := b.client.callText(ctx, cfg, strings.Join(lines, "\n"), img, b.maxTokensForModel(cfg))
	if err != nil {
		return "", err
	}
	analysis := extractJSONObject(responseText)
	if analysis == nil {
		return formatInvalidStructuredNote(responseText), nil
	}
	return formatStructuredVisionNote(analysis, caps), nil
}

func (b *Bridge) storeNote(imagePath string, entry *NoteEntry, sessionPath string) {
	b.mu.Lock()
	b.noteByPath[imagePath] = entry
	b.trimNoteCache()
	b.mu.Unlock()
	if sessionPath != "" {
		b.persistNote(sessionPath, imagePath, entry)
	}
}

func (b *Bridge) lookupNote(sessionPath, imagePath string) *NoteEntry {
	b.mu.Lock()
	memoryEntry := b.noteByPath[imagePath]
	b.mu.Unlock()
	if memoryEntry != nil && (sessionPath == "" || memoryEntry.SessionPath == "" || memoryEntry.SessionPath == sessionPath) {
		return memoryEntry
	}
	if sessionPath == "" {
		return nil
	}
	filePath := sessionNotesPath(sessionPath)
	sessionKey := sessionNotesKey(sessionPath)
	if filePath == "" || sessionKey == "" {
		return nil
	}
	sidecar, err := readNotesSidecar(filePath)
	if err != nil {
		return nil
	}
	sessionEntry, ok := sidecar.Sessions[sessionKey]
	if !ok {
		return nil
	}
	persisted, ok := sessionEntry.Images[imagePath]
	if !ok || persisted.Note == "" {
		return nil
	}
	restored := &NoteEntry{
		Note: persisted.Note, SessionPath: sessionPath, ImagePath: imagePath,
		UserRequest: persisted.UserRequest, VisionModel: persisted.VisionModel,
		TargetModel: persisted.TargetModel, UpdatedAt: persisted.UpdatedAt,
	}
	if restored.UpdatedAt == 0 {
		restored.UpdatedAt = b.now()
	}
	b.mu.Lock()
	b.noteByPath[imagePath] = restored
	b.trimNoteCache()
	b.mu.Unlock()
	return restored
}

func (b *Bridge) persistNote(sessionPath, imagePath string, entry *NoteEntry) {
	filePath := sessionNotesPath(sessionPath)
	sessionKey := sessionNotesKey(sessionPath)
	if filePath == "" || sessionKey == "" {
		return
	}
	sidecar, err := readNotesSidecar(filePath)
	if err != nil {
		return
	}
	sessionEntry := sidecar.Sessions[sessionKey]
	if sessionEntry.Images == nil {
		sessionEntry.Images = map[string]persistedNote{}
	}
	sessionEntry.Images[imagePath] = persistedNote{
		Note: entry.Note, ImagePath: imagePath, UserRequest: entry.UserRequest,
		VisionModel: entry.VisionModel, TargetModel: entry.TargetModel, UpdatedAt: entry.UpdatedAt,
	}
	sidecar.Sessions[sessionKey] = sessionEntry
	_ = writeNotesSidecar(filePath, sidecar)
}

func (b *Bridge) maxTokensForModel(cfg *ResolvedConfig) int {
	return b.visionMaxTokens
}

func (b *Bridge) trimAnalysisCache() {
	for len(b.analysisByPrompt) > b.maxCacheEntries {
		var oldestKey string
		var oldestAt int64
		first := true
		for k, v := range b.analysisByPrompt {
			if first || v.LastUsedAt < oldestAt {
				oldestKey, oldestAt, first = k, v.LastUsedAt, false
			}
		}
		delete(b.analysisByPrompt, oldestKey)
	}
}

func (b *Bridge) trimNoteCache() {
	if len(b.noteByPath) <= b.maxCacheEntries {
		return
	}
	// simple: delete arbitrary excess (session sidecar remains source of truth)
	for k := range b.noteByPath {
		if len(b.noteByPath) <= b.maxCacheEntries {
			break
		}
		delete(b.noteByPath, k)
	}
}

func imagePromptCacheKey(img ImageInput, userRequest, modelSignature string) string {
	h := sha256.New()
	h.Write([]byte(img.MimeType))
	h.Write([]byte{0})
	h.Write(img.Data)
	h.Write([]byte{0})
	h.Write([]byte(userRequest))
	h.Write([]byte{0})
	h.Write([]byte(modelSignature))
	return hex.EncodeToString(h.Sum(nil))
}

func visionModelCacheSignature(cfg *ResolvedConfig, caps *VisionCapabilities) string {
	payload := map[string]any{
		"provider": cfg.Provider,
		"id":       cfg.ModelID,
		"caps":     caps,
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

func orDefault(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return strings.TrimSpace(s)
}
