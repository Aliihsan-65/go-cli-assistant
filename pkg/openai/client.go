package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Client, OpenAI API ile etkileşim kurmak için bir istemcidir.
type Client struct {
	apiKey string
	model  string
	client *http.Client
}

// NewClient, yeni bir OpenAI istemcisi oluşturur.
func NewClient(apiKey string, model string) *Client {
	return &Client{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{},
	}
}

// APIRequest, OpenAI Chat Completions API'sine gönderilecek isteği temsil eder.
type APIRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

// Message, API isteği içindeki tek bir mesajı temsil eder.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// APIResponse, OpenAI API'sinden gelen yanıtı temsil eder.
type APIResponse struct {
	Choices []Choice `json:"choices"`
}

// Choice, API yanıtındaki bir seçim (cevap) seçeneğini temsil eder.
type Choice struct {
	Message Message `json:"message"`
}

// Generate, verilen bir prompt ile OpenAI'den bir yanıt üretir.
func (c *Client) Generate(prompt string) (string, error) {
	apiURL := "https://api.openai.com/v1/chat/completions"

	requestBody, err := json.Marshal(APIRequest{
		Model: c.model,
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("istek gövdesi oluşturulurken hata: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(requestBody))
	if err != nil {
		return "", fmt.Errorf("HTTP isteği oluşturulurken hata: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("OpenAI API'ye istek gönderilirken hata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenAI API'den başarısız yanıt (kod: %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "", fmt.Errorf("OpenAI API yanıtı okunurken hata: %w", err)
	}

	if len(apiResp.Choices) > 0 && apiResp.Choices[0].Message.Content != "" {
		return apiResp.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("OpenAI API'den boş veya geçersiz bir yanıt alındı")
}
