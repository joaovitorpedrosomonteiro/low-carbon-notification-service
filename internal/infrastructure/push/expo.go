package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

const expoPushURL = "https://exp.host/--/api/v2/push/send"

type ExpoPushRequest struct {
	To    string `json:"to"`
	Title string `json:"title"`
	Body  string `json:"body"`
	Sound string `json:"sound,omitempty"`
	Data  any    `json:"data,omitempty"`
}

type ExpoPushResponse struct {
	Data []struct {
		Status  string `json:"status"`
		ID      string `json:"id,omitempty"`
		Message string `json:"message,omitempty"`
	} `json:"data"`
}

type ExpoPushClient struct {
	httpClient *http.Client
}

func NewExpoPushClient() *ExpoPushClient {
	return &ExpoPushClient{
		httpClient: &http.Client{},
	}
}

func (c *ExpoPushClient) Send(ctx context.Context, token, title, body string) error {
	payload := ExpoPushRequest{
		To:    token,
		Title: title,
		Body:  body,
		Sound: "default",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal expo push: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, expoPushURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("expo push request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("expo push error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var pushResp ExpoPushResponse
	if err := json.Unmarshal(bodyBytes, &pushResp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	for _, result := range pushResp.Data {
		if result.Status == "error" {
			log.Printf("[Expo] Push error for token %s: %s", truncateToken(token), result.Message)
		}
	}

	log.Printf("[Expo] Push sent to %s: %s", truncateToken(token), title)
	return nil
}

type MockPushClient struct{}

func NewMockPushClient() *MockPushClient {
	return &MockPushClient{}
}

func (m *MockPushClient) Send(ctx context.Context, token, title, body string) error {
	log.Printf("[MockPush] Token: %s | Title: %s | Body: %s", truncateToken(token), title, body)
	return nil
}

func truncateToken(token string) string {
	if len(token) <= 12 {
		return token
	}
	return token[:8] + "..." + token[len(token)-4:]
}
