package vision

import (
	"context"
	"errors"
	"strings"

	"deepseek-anka/internal/i18n"
)

// PrepareInputOptions mirrors openhanako prepareVisionInputForTextOnlyModel.
type PrepareInputOptions struct {
	TargetModel   TargetModel
	Text          string
	ImagePaths    []string
	SessionPath   string
	WorkspaceRoot string
	Bridge        *Bridge
	Warn          func(string)
}

// PrepareInputResult is the prepared prompt text after auxiliary vision.
type PrepareInputResult struct {
	Text string
}

// PrepareInputForTextOnlyModel runs the openhanako vision-prepare flow for Reasonix
// text prompts that reference image attachment paths.
func PrepareInputForTextOnlyModel(ctx context.Context, opts PrepareInputOptions) (PrepareInputResult, error) {
	paths := opts.ImagePaths
	if len(paths) == 0 {
		paths = UniqueImagePathsFromText(opts.Text)
	}
	if len(paths) == 0 || !RequiresAuxiliaryVision(opts.TargetModel) {
		return PrepareInputResult{Text: opts.Text}, nil
	}
	if opts.Bridge == nil {
		return PrepareInputResult{}, errors.New("vision auxiliary model is required for image input with the current text-only model")
	}

	var resources []Resource
	for _, p := range paths {
		res, err := LoadImageResource(opts.WorkspaceRoot, p, p)
		if err != nil {
			if isRecoverableVisionPrepareError(err) {
				if opts.Warn != nil {
					opts.Warn("vision prepare failed, proceeding without images: " + err.Error())
				}
				return PrepareInputResult{Text: appendVisionFailureNotice(opts.Text, err)}, nil
			}
			return PrepareInputResult{}, err
		}
		resources = append(resources, res)
	}

	notes, err := opts.Bridge.PrepareResources(ctx, ResourcesOptions{
		SessionPath:   opts.SessionPath,
		WorkspaceRoot: opts.WorkspaceRoot,
		TargetModel:   opts.TargetModel,
		UserRequest:   opts.Text,
		Text:          opts.Text,
		Resources:     resources,
	})
	if err != nil {
		if isRecoverableVisionPrepareError(err) {
			if opts.Warn != nil {
				opts.Warn("vision prepare failed, proceeding without images: " + err.Error())
			}
			return PrepareInputResult{Text: appendVisionFailureNotice(opts.Text, err)}, nil
		}
		return PrepareInputResult{}, err
	}
	if len(notes) == 0 {
		return PrepareInputResult{Text: opts.Text}, nil
	}
	block := FormatVisionContext(notes)
	body := StripImageRefsFromText(opts.Text)
	if body == "" {
		return PrepareInputResult{Text: block}, nil
	}
	return PrepareInputResult{Text: block + body}, nil
}

func isRecoverableVisionPrepareError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "auth") || strings.Contains(msg, "401") || strings.Contains(msg, "403") {
		return false
	}
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "rate") || strings.Contains(msg, "empty") {
		return true
	}
	return strings.Contains(msg, "network") || strings.Contains(msg, "connection")
}

func appendVisionFailureNotice(text string, err error) string {
	notice := visionFailureNotice(err)
	if strings.TrimSpace(text) == "" {
		return notice
	}
	return notice + "\n\n" + text
}

func visionFailureNotice(err error) string {
	reason := ""
	if err != nil && err.Error() != "" {
		reason = " (" + err.Error() + ")"
	}
	if strings.HasPrefix(i18n.DetectLanguage(""), "zh") {
		return "[图片分析失败：辅助视觉模型暂时不可用，本轮不会把图片内容传给文本模型。请明确说明你没有看到图片，并请用户稍后重试或检查视觉模型配置" + reason + "。]"
	}
	return "[Image analysis failed: the auxiliary vision model is temporarily unavailable, so this turn does not include image content for the text-only model. Clearly state that you could not inspect the image, and ask the user to retry later or check the vision model configuration" + reason + ".]"
}
