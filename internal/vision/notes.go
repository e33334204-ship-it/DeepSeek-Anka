package vision

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type noteSidecar struct {
	Version  int                       `json:"version"`
	Sessions map[string]sessionNotes   `json:"sessions"`
}

type sessionNotes struct {
	Images map[string]persistedNote `json:"images"`
}

type persistedNote struct {
	Note        string    `json:"note"`
	ImagePath   string    `json:"imagePath"`
	UserRequest string    `json:"userRequest"`
	VisionModel *ModelRef `json:"visionModel"`
	TargetModel *ModelRef `json:"targetModel"`
	UpdatedAt   int64     `json:"updatedAt"`
}

// NoteEntry is an in-memory vision note keyed by image path.
type NoteEntry struct {
	Note        string
	SessionPath string
	ImagePath   string
	UserRequest string
	VisionModel *ModelRef
	TargetModel *ModelRef
	UpdatedAt   int64
}

func sessionNotesDir(sessionPath string) string {
	if sessionPath == "" {
		return ""
	}
	dir := filepath.Dir(sessionPath)
	if filepath.Base(dir) == "archived" {
		return filepath.Dir(dir)
	}
	return dir
}

func sessionNotesPath(sessionPath string) string {
	dir := sessionNotesDir(sessionPath)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, sessionNotesFile)
}

func sessionNotesKey(sessionPath string) string {
	if sessionPath == "" {
		return ""
	}
	return filepath.Base(sessionPath)
}

func emptyNotesSidecar() noteSidecar {
	return noteSidecar{Version: 1, Sessions: map[string]sessionNotes{}}
}

func readNotesSidecar(filePath string) (noteSidecar, error) {
	if filePath == "" {
		return emptyNotesSidecar(), nil
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyNotesSidecar(), nil
		}
		return noteSidecar{}, err
	}
	var parsed noteSidecar
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return emptyNotesSidecar(), nil
	}
	if parsed.Sessions == nil {
		parsed.Sessions = map[string]sessionNotes{}
	}
	if parsed.Version == 0 {
		parsed.Version = 1
	}
	return parsed, nil
}

func writeNotesSidecar(filePath string, data noteSidecar) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.tmp-%d-%d", filePath, os.Getpid(), time.Now().UnixNano())
	payload, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(tmp, payload, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filePath)
}
