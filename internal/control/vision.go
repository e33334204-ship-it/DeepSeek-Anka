package control

import (
	"context"
	"fmt"

	"deepseek-anka/internal/capabilities"
	"deepseek-anka/internal/provider"
	"deepseek-anka/internal/vision"
)

func (c *Controller) targetModel() vision.TargetModel {
	if cfg := capabilities.Config(); cfg != nil {
		return vision.TargetModelFromConfig(cfg)
	}
	return vision.TargetModel{}
}

func (c *Controller) prepareVisionForInput(ctx context.Context, input string) (string, []string) {
	br := capabilities.VisionBridge()
	if br == nil || !capabilities.VisionEnabled() {
		return input, nil
	}
	target := c.targetModel()
	if !vision.RequiresAuxiliaryVision(target) {
		return input, nil
	}
	paths := vision.UniqueImagePathsFromText(input)
	if len(paths) == 0 {
		return input, nil
	}
	result, err := vision.PrepareInputForTextOnlyModel(ctx, vision.PrepareInputOptions{
		TargetModel:   target,
		Text:          input,
		ImagePaths:    paths,
		SessionPath:   c.sessionPath,
		WorkspaceRoot: c.cpRoot,
		Bridge:        br,
		Warn:          func(msg string) { c.notice(msg) },
	})
	if err != nil {
		c.notice(err.Error())
		return input, nil
	}
	return result.Text, paths
}

// makeVisionPipelineMutator returns a PreStreamMutator that runs the visual
// context pipeline on all messages before each LLM stream. Returns nil when
// vision is disabled, so the agent never allocates for a no-op.
func (c *Controller) makeVisionPipelineMutator() func(ctx context.Context, msgs []provider.Message) ([]provider.Message, error) {
	if !capabilities.VisionEnabled() {
		return nil
	}
	br := capabilities.VisionBridge()
	if br == nil {
		return nil
	}
	return func(ctx context.Context, msgs []provider.Message) ([]provider.Message, error) {
		target := vision.TargetModelFromConfig(capabilities.Config())
		if !vision.RequiresAuxiliaryVision(target) {
			return msgs, nil
		}
		sp := c.sessionPath // read at call time — may be set after creation
		result, injected, err := vision.AdaptVisualContextMessages(ctx, msgs, target, vision.PipelineOptions{
			SessionPath:   sp,
			WorkspaceRoot: c.cpRoot,
			Bridge:        br,
			Warn:          func(msg string) { c.notice(msg) },
		})
		if err != nil {
			return msgs, fmt.Errorf("vision pipeline: %w", err)
		}
		if injected > 0 {
			return result, nil
		}
		return msgs, nil
	}
}
