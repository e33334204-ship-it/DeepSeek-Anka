// Package vision provides visual context analysis and injection for text-only
// models. The pipeline (AdaptVisualContextMessages) scans conversation messages
// for image references, loads and deduplicates image resources, analyzes them
// via the auxiliary vision Bridge, and injects structured notes back into
// messages as <vision-context> blocks — ported from openhanako's
// visual-context-pipeline.js.
package vision

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"deepseek-anka/internal/provider"
)

// VisualResource is a keyed image resource for vision analysis.
type VisualResource struct {
	Key         string
	Label       string
	ContentHash string
	Image       ImageInput
}

// PipelineOptions carries dependencies for the visual context pipeline.
type PipelineOptions struct {
	SessionPath   string
	WorkspaceRoot string
	Bridge        *Bridge
	Warn          func(string)
}

// ImageResourceKey creates a unique resource key for an image based on its
// MIME type and content hash — equivalent to openhanako's
// visualResourceKeyForImage.
func ImageResourceKey(img ImageInput, contentHash string) string {
	h := sha256.Sum256([]byte(img.MimeType + "\x00" + contentHash))
	return "visual-resource:image:" + hex.EncodeToString(h[:])
}

// SessionFileResourceKey creates a unique resource key for a session file
// — equivalent to openhanako's visualResourceKeyForSessionFile.
func SessionFileResourceKey(file SessionFileMeta, contentHash string) string {
	h := sha256.Sum256([]byte(
		file.ID + "\x00" +
			file.SessionPath + "\x00" +
			file.RealPath + "\x00" +
			file.Mime + "\x00" +
			contentHash,
	))
	return "visual-resource:session-file:" + hex.EncodeToString(h[:])
}

// SessionFileMeta carries identifying metadata for a session file.
type SessionFileMeta struct {
	ID          string
	SessionPath string
	RealPath    string
	Mime        string
	Kind        string
	Label       string
	Filename    string
	Status      string
}

// resourceFromImageBlock creates a VisualResource from an image content block.
func resourceFromImageBlock(img ImageInput, index int) *VisualResource {
	if len(img.Data) == 0 {
		return nil
	}
	mime := img.MimeType
	if mime == "" {
		mime = "image/png"
	}
	hash := sha256.Sum256(img.Data)
	contentHash := hex.EncodeToString(hash[:])
	return &VisualResource{
		Key:         ImageResourceKey(img, contentHash),
		Label:       fmt.Sprintf("image %d", index+1),
		ContentHash: contentHash,
		Image:       ImageInput{Data: img.Data, MimeType: mime},
	}
}

// resourceFromFile loads a file from disk and creates a VisualResource.
func resourceFromFile(workspaceRoot, filePath string) (*VisualResource, error) {
	abs, err := ResolveAttachmentPath(workspaceRoot, filePath)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty file: %s", filePath)
	}
	mime := detectMIME(data, abs)
	hash := sha256.Sum256(data)
	contentHash := hex.EncodeToString(hash[:])
	key := filepath.ToSlash(filePath)
	if key == "" {
		key = ImageResourceKey(ImageInput{MimeType: mime}, contentHash)
	}
	return &VisualResource{
		Key:         key,
		Label:       filepath.Base(abs),
		ContentHash: contentHash,
		Image:       ImageInput{Data: data, MimeType: mime},
	}, nil
}

// collectMessageResources scans a user message for embedded image references
// and returns the corresponding VisualResources. It handles both
// [attached_image: path] markers and @.deepseek-anka/attachments/ path refs.
func collectMessageResources(msg provider.Message, workspaceRoot string) []VisualResource {
	if msg.Role != provider.RoleUser || msg.Content == "" {
		return nil
	}
	text := msg.Content
	if strings.Contains(text, ContextStart) {
		return nil // already has vision context injected
	}
	paths := UniqueImagePathsFromText(text)
	if len(paths) == 0 {
		return nil
	}
	var resources []VisualResource
	for _, p := range paths {
		res, err := resourceFromFile(workspaceRoot, p)
		if err != nil {
			continue // skip unresolvable files
		}
		resources = append(resources, *res)
	}
	return resources
}

// dedupeResources deduplicates a slice of VisualResources by content hash.
func dedupeResources(resources []VisualResource) []VisualResource {
	seen := map[string]bool{}
	var out []VisualResource
	for _, r := range resources {
		key := r.ContentHash
		if key == "" {
			key = r.Key
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}

// perMessageResources collects resources from each user message, preserving
// the per-message grouping. Returns the per-message lists and the globally
// deduplicated flat list (matching openhanako's dual byMessage+allResources).
func perMessageResources(messages []provider.Message, workspaceRoot string) ([][]VisualResource, []VisualResource) {
	byMessage := make([][]VisualResource, len(messages))
	var all []VisualResource
	for i, msg := range messages {
		res := collectMessageResources(msg, workspaceRoot)
		unique := dedupeResources(res)
		byMessage[i] = unique
		all = append(all, unique...)
	}
	return byMessage, dedupeResources(all)
}

// injectMessageNotes injects vision context notes into a user message.
// Returns the modified message and whether any notes were injected.
func injectMessageNotes(msg provider.Message, notes []PreparedNote) (provider.Message, bool) {
	if msg.Role != provider.RoleUser || len(notes) == 0 {
		return msg, false
	}
	if strings.Contains(msg.Content, ContextStart) {
		return msg, false // already has vision context
	}
	block := FormatVisionContext(notes)
	if block == "" {
		return msg, false
	}
	msg.Content = block + msg.Content
	return msg, true
}

// AdaptVisualContextMessages scans all user messages for image references,
// loads and deduplicates image resources, analyzes them via the vision Bridge,
// and injects structured notes back into the messages. Only processes when the
// target model requires auxiliary vision. Ported from openhanako's
// visual-context-pipeline.js adaptVisualContextMessages().
//
// Returns the modified messages and the count of injected notes.
func AdaptVisualContextMessages(ctx context.Context, messages []provider.Message, target TargetModel, opts PipelineOptions) ([]provider.Message, int, error) {
	if opts.Bridge == nil {
		return messages, 0, nil
	}
	if !RequiresAuxiliaryVision(target) {
		return messages, 0, nil
	}

	// First pass: collect per-message resources, keep global deduplicated set.
	byMessage, allResources := perMessageResources(messages, opts.WorkspaceRoot)
	if len(allResources) == 0 {
		return messages, 0, nil
	}

	// Convert to Bridge's Resource type and analyze.
	bridgeResources := make([]Resource, len(allResources))
	for i, r := range allResources {
		bridgeResources[i] = Resource{
			Key:   r.Key,
			Label: r.Label,
			Image: r.Image,
		}
	}

	userRequest := userRequestFromMessages(messages)
	prepared, err := opts.Bridge.PrepareResources(ctx, ResourcesOptions{
		SessionPath:   opts.SessionPath,
		WorkspaceRoot: opts.WorkspaceRoot,
		TargetModel:   target,
		UserRequest:   userRequest,
		Resources:     bridgeResources,
	})
	if err != nil {
		if opts.Warn != nil {
			opts.Warn("vision pipeline: " + err.Error())
		}
		return messages, 0, nil // non-fatal: proceed without vision
	}

	// Build a lookup by resource key.
	notesByKey := map[string]PreparedNote{}
	for _, n := range prepared {
		notesByKey[n.Key] = n
	}

	// Second pass: map per-message resources to prepared notes and inject.
	injected := 0
	result := make([]provider.Message, len(messages))
	copy(result, messages)
	for i := range messages {
		if len(byMessage[i]) == 0 {
			continue // no resources in this message
		}
		var msgNotes []PreparedNote
		for _, res := range byMessage[i] {
			if note, ok := notesByKey[res.Key]; ok {
				msgNotes = append(msgNotes, note)
			}
		}
		if len(msgNotes) == 0 {
			continue
		}
		modified, ok := injectMessageNotes(result[i], msgNotes)
		if ok {
			result[i] = modified
			injected += len(msgNotes)
		}
	}

	if injected > 0 {
		return result, injected, nil
	}
	return messages, 0, nil
}

// userRequestFromMessages extracts the last user message text from a
// conversation, stripping vision context and image markers.
func userRequestFromMessages(messages []provider.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == provider.RoleUser {
			text := messages[i].Content
			if text == "" {
				continue
			}
			return normalizeUserRequest(text)
		}
	}
	return ""
}
