package ollama

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// --- Yeni Yapılar ---

// AIResponse, yapay zekadan gelen tüm cevap türlerini modelleyen ana yapıdır.
type AIResponse struct {
	Type          string   `json:"type"`
	Conversation  string   `json:"conversation,omitempty"`
	ToolCall      ToolCall `json:"tool_call,omitempty"`
	Clarification string   `json:"clarification,omitempty"`
	Options       []string `json:"options,omitempty"`
}

// ToolCall, LLM'in bir araç kullanma isteğini temsil eder.
// Bu artık AIResponse'un bir parçası.
type ToolCall struct {
	ToolName string            `json:"tool_name"`
	Params   map[string]string `json:"params"`
}


// --- Mevcut Fonksiyonlar (Değişiklik Yok) ---

// Request, Ollama API'sine gönderilecek isteği temsil eder.
type Request struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// Ollama API'sinden gelen ham cevabı temsil eder.
type ollamaRawResponse struct {
	Response string `json:"response"`
}

// Generate, Ollama API'sine bir istek gönderir ve ham metin cevabını alır.
func Generate(apiURL, model, prompt string) (string, error) {
	requestData := Request{
		Model:  model,
		Prompt: prompt,
		Stream: false,
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return "", fmt.Errorf("JSON marshal hatası: %w", err)
	}

	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("Ollama API hatası: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("cevap okuma hatası: %w", err)
	}

	var ollamaResp ollamaRawResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		// Ham cevabı doğrudan döndürmeyi dene, belki JSON değildir.
		return string(body), fmt.Errorf("Ollama cevap JSON'u çözümlenemedi: %w", err)
	}

	return ollamaResp.Response, nil
}