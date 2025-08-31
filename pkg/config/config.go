package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// Config, uygulamanın tüm yapılandırmasını tutar.
type Config struct {
	Ollama struct {
		Model string `yaml:"model"`
		URL   string `yaml:"url"`
	} `yaml:"ollama"`
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
