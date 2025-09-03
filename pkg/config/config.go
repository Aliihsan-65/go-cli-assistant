package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// Config, uygulamanın tüm yapılandırmasını tutar.
type Config struct {
	Ollama struct {
		Model          string `yaml:"model"`
		EmbeddingModel string `yaml:"embedding_model"`
		URL            string `yaml:"url"`
	} `yaml:"ollama"`
	Chroma struct {
		URL                 string  `yaml:"url"`
		CollectionName      string  `yaml:"collection_name"`
		SimilarityThreshold float64 `yaml:"similarity_threshold"`
	} `yaml:"chroma"`
	ExpertAPI struct {
		APIKey string `yaml:"api_key"`
	} `yaml:"expert_api"`
}

// LoadConfig, belirtilen yoldan yapılandırma dosyasını okur ve Config struct'ını doldurur.
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		path = "config.yaml"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("yapılandırma dosyası okunamadı: %w", err)
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, fmt.Errorf("yapılandırma dosyası çözümlenemedi: %w", err)
	}

	return &cfg, nil
}
