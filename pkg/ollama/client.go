package ollama

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// --- Mevcut Fonksiyonlar ---

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
	// apiURL artık ana adresi içeriyor, endpoint'i burada ekliyoruz.
	fullURL := apiURL + "/api/generate"

	requestData := Request{
		Model:  model,
		Prompt: prompt,
		Stream: false,
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return "", fmt.Errorf("JSON marshal hatası: %w", err)
	}

	resp, err := http.Post(fullURL, "application/json", bytes.NewBuffer(jsonData))
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

// --- YENİ EMBEDDING FONKSİYONLARI ---

// EmbeddingRequest, Ollama'nın embedding API'sine gönderilecek isteği temsil eder.
type EmbeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// EmbeddingResponse, Ollama'nın embedding API'sinden gelen cevabı temsil eder.
type EmbeddingResponse struct {
	Embedding []float32 `json:"embedding"`
}

// GenerateEmbedding, bir metni Ollama aracılığıyla bir vektöre dönüştürür.
func GenerateEmbedding(apiURL, model, prompt string) ([]float32, error) {
	// apiURL artık ana adresi içeriyor, endpoint'i burada ekliyoruz.
	fullURL := apiURL + "/api/embeddings"

	requestData := EmbeddingRequest{
		Model:  model,
		Prompt: prompt,
	}

	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("embedding request JSON marshal hatası: %w", err)
	}

	resp, err := http.Post(fullURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("Ollama embedding API hatası: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("embedding cevap okuma hatası: %w", err)
	}

	var embeddingResp EmbeddingResponse
	if err := json.Unmarshal(body, &embeddingResp); err != nil {
		return nil, fmt.Errorf("Ollama embedding cevap JSON'u çözümlenemedi: %w, Gelen Cevap: %s", err, string(body))
	}

	return embeddingResp.Embedding, nil
}
