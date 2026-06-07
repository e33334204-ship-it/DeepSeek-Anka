package vision

import (
	"context"
	"errors"
	"strings"
	"testing"

	"deepseek-anka/internal/config"
)

func TestPrepareInputWithoutBridgeStripsImageRefs(t *testing.T) {
	target := TargetModel{
		Ref: ModelRef{ID: "deepseek-chat", Provider: "deepseek"},
		Entry: &config.ProviderEntry{
			Name: "deepseek", Kind: "openai", BaseURL: "https://api.deepseek.com", Model: "deepseek-chat",
		},
	}
	result, err := PrepareInputForTextOnlyModel(context.Background(), PrepareInputOptions{
		TargetModel: target,
		Text:        "@.deepseek-anka/attachments/clipboard.png\n描述一下这张图片",
		ImagePaths:  []string{".deepseek-anka/attachments/clipboard.png"},
	})
	if err != nil {
		t.Fatalf("PrepareInputForTextOnlyModel returned error: %v", err)
	}
	if strings.Contains(result.Text, ".deepseek-anka/attachments") {
		t.Fatalf("prepared text leaked attachment path: %q", result.Text)
	}
	if !strings.Contains(result.Text, "描述一下这张图片") {
		t.Fatalf("prepared text lost user request: %q", result.Text)
	}
	if !strings.Contains(result.Text, "图片分析失败") && !strings.Contains(result.Text, "Image analysis failed") {
		t.Fatalf("prepared text missing failure notice: %q", result.Text)
	}
}

func TestAppendFailureNoticeDoesNotReintroduceStrippedRefs(t *testing.T) {
	text := StripImageRefsFromText("@.deepseek-anka/attachments/a.png\n看看这个")
	got := AppendFailureNoticeForText(text, errors.New("vision API 401: invalid authentication"))
	if strings.Contains(got, ".deepseek-anka/attachments") {
		t.Fatalf("failure notice leaked attachment path: %q", got)
	}
	if !strings.Contains(got, "看看这个") {
		t.Fatalf("failure notice lost stripped text: %q", got)
	}
}
