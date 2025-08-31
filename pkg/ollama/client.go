package ollama

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Request, Ollama API'sine gönderilecek isteği temsil eder.
type Request struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// Response, Ollama API'sinden gelen cevabı temsil eder.
type Response struct {
	Response string `json:"response"`
}

// ToolCall, LLM'in bir araç kullanma isteğini temsil eder.
type ToolCall struct {
	ToolName string            `json:"tool_name"`
	Params   map[string]string `json:"params"`
}

// Generate, Ollama API'sine bir istek gönderir ve cevabı alır.
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

	var ollamaResp Response
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return "", fmt.Errorf("Ollama cevap JSON'u çözümlenemedi: %w", err)
	}

	return ollamaResp.Response, nil
}
