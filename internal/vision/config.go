package vision

import "reasonix/internal/config"

// ResolveVisionConfig returns auxiliary vision model credentials (openhanako resolveVisionConfig).
func ResolveVisionConfig(cfg *config.Config) (*ResolvedConfig, error) {
	if cfg == nil {
		return nil, nil
	}
	entry, err := cfg.ResolveVisionModel()
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}
	modelID := entry.Model
	if modelID == "" {
		modelID = entry.DefaultModel()
	}
	api := entry.Kind
	if api == "" {
		api = "openai"
	}
	return &ResolvedConfig{
		Model:    TargetModel{Ref: ModelRef{ID: modelID, Provider: entry.Name}, Entry: entry},
		Entry:    entry,
		Provider: entry.Name,
		ModelID:  modelID,
		API:      api,
		APIKey:   entry.APIKey(),
		BaseURL:  entry.BaseURL,
	}, nil
}
