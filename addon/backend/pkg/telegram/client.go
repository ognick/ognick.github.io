package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

type Client interface {
	DownloadFile(ctx context.Context, fileID string) ([]byte, error)
	SendMessage(ctx context.Context, chatID int, text string) error
}

type telegramClient struct {
	botToken string
	client   *http.Client
}

func NewClient(botToken string) Client {
	return &telegramClient{botToken: botToken, client: http.DefaultClient}
}

func (c *telegramClient) baseURL() string {
	return "https://api.telegram.org/bot" + c.botToken
}

func (c *telegramClient) DownloadFile(ctx context.Context, fileID string) ([]byte, error) {
	filePath, err := c.getFilePath(ctx, fileID)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", c.botToken, filePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build download request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download file: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (c *telegramClient) getFilePath(ctx context.Context, fileID string) (string, error) {
	url := c.baseURL() + "/getFile?file_id=" + fileID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build getFile request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("getFile: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("getFile returned %d", resp.StatusCode)
	}

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode getFile response: %w", err)
	}
	if !result.OK {
		return "", fmt.Errorf("getFile: telegram api returned ok=false")
	}
	return result.Result.FilePath, nil
}

func (c *telegramClient) SendMessage(ctx context.Context, chatID int, text string) error {
	url := c.baseURL() + "/sendMessage"

	body := map[string]string{
		"chat_id": strconv.Itoa(chatID),
		"text":    text,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal sendMessage: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build sendMessage request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("sendMessage: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sendMessage returned %d", resp.StatusCode)
	}
	return nil
}
