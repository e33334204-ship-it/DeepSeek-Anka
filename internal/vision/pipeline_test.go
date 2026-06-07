package vision

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"deepseek-anka/internal/provider"
)

func TestImageResourceKey(t *testing.T) {
	img := ImageInput{MimeType: "image/png", Data: []byte("test-data")}
	hash := sha256.Sum256([]byte("test-data"))
	contentHash := hex.EncodeToString(hash[:])

	key := ImageResourceKey(img, contentHash)
	if !strings.HasPrefix(key, "visual-resource:image:") {
		t.Errorf("key should start with visual-resource:image:, got %q", key)
	}
	if len(key) != len("visual-resource:image:")+64 {
		t.Errorf("key should be 64 hex chars after prefix, got %q (len=%d)", key, len(key))
	}

	// Deterministic: same inputs → same key
	key2 := ImageResourceKey(img, contentHash)
	if key != key2 {
		t.Errorf("key should be deterministic: %q != %q", key, key2)
	}
}

func TestSessionFileResourceKey(t *testing.T) {
	file := SessionFileMeta{
		ID:          "file-123",
		SessionPath: "/sessions/session-1.jsonl",
		RealPath:    "/data/img.png",
		Mime:        "image/png",
	}
	key := SessionFileResourceKey(file, "abc123")
	if !strings.HasPrefix(key, "visual-resource:session-file:") {
		t.Errorf("key should start with visual-resource:session-file:, got %q", key)
	}
}

func TestResourceFromFile(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "test.png")
	if err := os.WriteFile(imgPath, []byte("fake-png-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := resourceFromFile("", imgPath)
	if err != nil {
		t.Fatalf("resourceFromFile failed: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil resource")
	}
	if res.Key == "" {
		t.Error("expected non-empty key")
	}
	if want := filepath.ToSlash(imgPath); res.Key != want {
		t.Errorf("expected path cache key %q, got %q", want, res.Key)
	}
	if res.Label != "test.png" {
		t.Errorf("expected label 'test.png', got %q", res.Label)
	}
	if len(res.Image.Data) == 0 {
		t.Error("expected non-empty image data")
	}
}

func TestResourceFromFile_NotExist(t *testing.T) {
	_, err := resourceFromFile("", "/nonexistent/path/img.png")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestCollectMessageResources(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "photo.png")
	if err := os.WriteFile(imgPath, []byte("photo-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		msg     provider.Message
		wantLen int
	}{
		{
			name:    "non-user message",
			msg:     provider.Message{Role: provider.RoleAssistant, Content: "hello"},
			wantLen: 0,
		},
		{
			name:    "empty content",
			msg:     provider.Message{Role: provider.RoleUser, Content: ""},
			wantLen: 0,
		},
		{
			name:    "no image refs",
			msg:     provider.Message{Role: provider.RoleUser, Content: "what is the weather?"},
			wantLen: 0,
		},
		{
			name:    "attached_image marker",
			msg:     provider.Message{Role: provider.RoleUser, Content: "[attached_image: " + imgPath + "]"},
			wantLen: 1,
		},
		{
			name:    "already has vision context",
			msg:     provider.Message{Role: provider.RoleUser, Content: ContextStart + "\nnote\n" + ContextEnd + "\n\nhello"},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resources := collectMessageResources(tt.msg, "")
			if len(resources) != tt.wantLen {
				t.Errorf("got %d resources, want %d", len(resources), tt.wantLen)
			}
		})
	}
}

func TestDedupeResources(t *testing.T) {
	r1 := VisualResource{
		Key:         "key-1",
		ContentHash: "hash-abc",
		Image:       ImageInput{Data: []byte("data1"), MimeType: "image/png"},
	}
	r2 := VisualResource{
		Key:         "key-2",
		ContentHash: "hash-abc", // same content hash as r1
		Image:       ImageInput{Data: []byte("data1"), MimeType: "image/png"},
	}
	r3 := VisualResource{
		Key:         "key-3",
		ContentHash: "hash-xyz", // different content hash
		Image:       ImageInput{Data: []byte("data2"), MimeType: "image/png"},
	}

	got := dedupeResources([]VisualResource{r1, r2, r3})
	if len(got) != 2 {
		t.Errorf("expected 2 deduplicated resources, got %d", len(got))
	}
}

func TestPerMessageResources(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "a.png")
	if err := os.WriteFile(imgPath, []byte("img-a"), 0o644); err != nil {
		t.Fatal(err)
	}

	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "[attached_image: " + imgPath + "] what is this?"},
		{Role: provider.RoleAssistant, Content: "I see an image."},
		{Role: provider.RoleUser, Content: "just text"},
	}
	byMessage, all := perMessageResources(msgs, "")
	if len(byMessage) != 3 {
		t.Errorf("expected 3 by-message entries, got %d", len(byMessage))
	}
	if len(byMessage[0]) != 1 {
		t.Errorf("expected 1 resource in message 0, got %d", len(byMessage[0]))
	}
	if len(byMessage[1]) != 0 {
		t.Errorf("expected 0 resources in message 1 (assistant), got %d", len(byMessage[1]))
	}
	if len(byMessage[2]) != 0 {
		t.Errorf("expected 0 resources in message 2 (no refs), got %d", len(byMessage[2]))
	}
	if len(all) != 1 {
		t.Errorf("expected 1 unique resource globally, got %d", len(all))
	}
}

func TestInjectMessageNotes(t *testing.T) {
	msg := provider.Message{Role: provider.RoleUser, Content: "hello"}
	notes := []PreparedNote{
		{Key: "res-1", Note: "a photo of a cat"},
	}

	modified, ok := injectMessageNotes(msg, notes)
	if !ok {
		t.Error("expected injection to succeed")
	}
	if !strings.Contains(modified.Content, ContextStart) {
		t.Error("expected vision context start marker")
	}
	if !strings.Contains(modified.Content, "a photo of a cat") {
		t.Error("expected note text in content")
	}

	// Should not inject again if already present
	_, ok2 := injectMessageNotes(modified, notes)
	if ok2 {
		t.Error("should not inject into already-injected message")
	}
}

func TestInjectMessageNotes_NonUser(t *testing.T) {
	msg := provider.Message{Role: provider.RoleAssistant, Content: "hello"}
	_, ok := injectMessageNotes(msg, []PreparedNote{{Key: "k", Note: "note"}})
	if ok {
		t.Error("should not inject into assistant messages")
	}
}

func TestUserRequestFromMessages(t *testing.T) {
	tests := []struct {
		name string
		msgs []provider.Message
		want string
	}{
		{
			name: "last user message",
			msgs: []provider.Message{
				{Role: provider.RoleUser, Content: "first"},
				{Role: provider.RoleAssistant, Content: "response"},
				{Role: provider.RoleUser, Content: "last request"},
			},
			want: "last request",
		},
		{
			name: "strips image markers",
			msgs: []provider.Message{
				{Role: provider.RoleUser, Content: "[attached_image: foo.png] what is this?"},
			},
			want: "what is this?",
		},
		{
			name: "empty messages",
			msgs: []provider.Message{},
			want: "",
		},
		{
			name: "only assistant messages",
			msgs: []provider.Message{
				{Role: provider.RoleAssistant, Content: "hello"},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := userRequestFromMessages(tt.msgs)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAdaptVisualContextMessages_NoBridge(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "hello"},
	}
	result, injected, err := AdaptVisualContextMessages(context.Background(), msgs, TargetModel{}, PipelineOptions{
		Bridge: nil,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if injected != 0 {
		t.Errorf("expected 0 injected, got %d", injected)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 message, got %d", len(result))
	}
}

func TestAdaptVisualContextMessages_NoAuxiliaryVision(t *testing.T) {
	// A model that supports images directly (not text-only) should skip the pipeline.
	// With a nil Entry, RequiresAuxiliaryVision returns false, so the pipeline skips.
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "hello"},
	}
	result, injected, err := AdaptVisualContextMessages(context.Background(), msgs, TargetModel{}, PipelineOptions{
		Bridge: &Bridge{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if injected != 0 {
		t.Errorf("expected 0 injected, got %d", injected)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 message, got %d", len(result))
	}
}

func TestAdaptVisualContextMessages_NoImages(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "just text, no images"},
	}
	result, injected, err := AdaptVisualContextMessages(context.Background(), msgs, TargetModel{}, PipelineOptions{
		Bridge: &Bridge{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if injected != 0 {
		t.Errorf("expected 0 injected, got %d", injected)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 message, got %d", len(result))
	}
}
