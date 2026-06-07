package visionllm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Client interface {
	ChatCompletion(ctx context.Context, messages []Message) (string, error)
}

type Message struct {
	Role    string
	Content []ContentPart
}

type ContentPart struct {
	Type     string
	Text     string
	ImageURL string
}

type openAIClient struct {
	baseURL string
	apiKey  string
	model   string
}

func NewClient(baseURL, apiKey, model string) Client {
	return &openAIClient{baseURL: baseURL, apiKey: apiKey, model: model}
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type imageContent struct {
	Type     string    `json:"type"`
	ImageURL *imageURL `json:"image_url"`
}

type imageURL struct {
	URL string `json:"url"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *openAIClient) ChatCompletion(ctx context.Context, messages []Message) (string, error) {
	chatMsgs := make([]chatMessage, 0, len(messages))
	for _, m := range messages {
		parts := make([]interface{}, 0, len(m.Content))
		for _, p := range m.Content {
			switch p.Type {
			case "text":
				parts = append(parts, textContent{Type: "text", Text: p.Text})
			case "image_url":
				parts = append(parts, imageContent{Type: "image_url", ImageURL: &imageURL{URL: p.ImageURL}})
			}
		}
		if len(parts) == 1 {
			if t, ok := parts[0].(textContent); ok {
				chatMsgs = append(chatMsgs, chatMessage{Role: m.Role, Content: t.Text})
				continue
			}
		}
		chatMsgs = append(chatMsgs, chatMessage{Role: m.Role, Content: parts})
	}

	req := chatRequest{Model: c.model, Messages: chatMsgs}

	data, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(c.baseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("api call: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm returned %d", resp.StatusCode)
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("empty response")
	}

	return chatResp.Choices[0].Message.Content, nil
}
