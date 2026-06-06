package vision

import "deepseek-anka/internal/config"

// ModelRef mirrors openhanako's shared model reference {id, provider}.
type ModelRef struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
}

// TargetModel describes the active chat model for auxiliary-vision gating.
type TargetModel struct {
	Ref   ModelRef
	Entry *config.ProviderEntry
}

// ResolvedConfig is vision auxiliary model + credentials (openhanako resolveVisionConfig).
type ResolvedConfig struct {
	Model    TargetModel
	Entry    *config.ProviderEntry
	Provider string
	ModelID  string
	API      string
	APIKey   string
	BaseURL  string
}

func compactModelRef(entry *config.ProviderEntry) *ModelRef {
	if entry == nil {
		return nil
	}
	id := entry.Model
	if id == "" {
		id = entry.DefaultModel()
	}
	if id == "" || entry.Name == "" {
		return nil
	}
	return &ModelRef{ID: id, Provider: entry.Name}
}

func targetFromEntry(entry *config.ProviderEntry) TargetModel {
	ref := ModelRef{}
	if entry != nil {
		ref.Provider = entry.Name
		ref.ID = entry.Model
		if ref.ID == "" {
			ref.ID = entry.DefaultModel()
		}
	}
	return TargetModel{Ref: ref, Entry: entry}
}
