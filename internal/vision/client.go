package vision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"deepseek-anka/internal/netclient"
)

type visionClient struct {
	http *http.Client
}

func newVisionClient(proxy netclient.ProxySpec) (*visionClient, error) {
	client, err := netclient.NewHTTPClient(proxy, netclient.TransportOptions{
		ResponseHeaderTimeout: analysisTimeout * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return &visionClient{http: client}, nil
}

func (c *visionClient) callText(ctx context.Context, cfg *ResolvedConfig, prompt string, img ImageInput, maxTokens int) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, analysisTimeout*time.Second)
	defer cancel()

	b64 := base64.StdEncoding.EncodeToString(img.Data)
	modelID := cfg.ModelID
	base := strings.TrimRight(cfg.BaseURL, "/")

	var body map[string]any
	switch strings.ToLower(cfg.API) {
	case "anthropic":
		body = map[string]any{
			"model":      modelID,
			"max_tokens": maxTokens,
			"messages": []map[string]any{{
				"role": "user",
				"content": []map[string]any{
					{"type": "text", "text": prompt},
					{"type": "image", "source": map[string]string{
						"type":       "base64",
						"media_type": img.MimeType,
						"data":       b64,
					}},
				},
			}},
		}
	default:
		body = map[string]any{
			"model": modelID,
			"messages": []map[string]any{{
				"role": "user",
				"content": []map[string]any{
					{"type": "text", "text": prompt},
					{"type": "image_url", "image_url": map[string]string{
						"url": "data:" + img.MimeType + ";base64," + b64,
					}},
				},
			}},
			"max_tokens": maxTokens,
		}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	endpoint := "/chat/completions"
	if strings.ToLower(cfg.API) == "anthropic" {
		endpoint = "/v1/messages"
		if strings.HasSuffix(base, "/v1") {
			base = strings.TrimSuffix(base, "/v1")
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		if strings.ToLower(cfg.API) == "anthropic" {
			req.Header.Set("x-api-key", cfg.APIKey)
			req.Header.Set("anthropic-version", "2023-06-01")
		} else {
			req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		}
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("vision API %d: %s", resp.StatusCode, truncateText(string(respBody), 400))
	}

	if strings.ToLower(cfg.API) == "anthropic" {
		var parsed struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			return "", fmt.Errorf("parse vision response: %w", err)
		}
		var parts []string
		for _, block := range parsed.Content {
			if t := strings.TrimSpace(block.Text); t != "" {
				parts = append(parts, t)
			}
		}
		if len(parts) == 0 {
			return "", fmt.Errorf("vision model returned empty content")
		}
		return strings.Join(parts, "\n"), nil
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("parse vision response: %w", err)
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("vision model returned empty content")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}
