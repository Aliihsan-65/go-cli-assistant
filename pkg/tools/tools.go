package tools

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Tool, bir aracı ve onunla ilgili AI'ya yol gösterecek meta verileri tanımlar.
type Tool struct {
	Name        string
	Description string
	Execute     func(params map[string]string) (string, error)
}

// ToolRegistry, sistemdeki tüm araçları tanımlarıyla birlikte tutar.
var ToolRegistry = map[string]Tool{
	"list_directory": {
		Name:        "list_directory",
		Description: "Belirtilen bir dosya sistemi dizinindeki dosyaları ve klasörleri listeler. Parametreler: { \"file_path\": \"string\" }",
		Execute: func(params map[string]string) (string, error) {
			path := params["file_path"]
			if path == "" {
				path = "."
			}
			files, err := os.ReadDir(path)
			if err != nil {
				return "", fmt.Errorf("dizin okunamadı: %w", err)
			}
			var fileNames []string
			for _, file := range files {
				if file.IsDir() {
					fileNames = append(fileNames, file.Name()+"/")
				} else {
					fileNames = append(fileNames, file.Name())
				}
			}
			return "Dizin içeriği:\n" + strings.Join(fileNames, "\n"), nil
		},
	},
	"read_file": {
		Name:        "read_file",
		Description: "Belirtilen bir dosyanın içeriğini okur. Parametreler: { \"file_path\": \"string\" }",
		Execute: func(params map[string]string) (string, error) {
			path := params["file_path"]
			if path == "" {
				return "", fmt.Errorf("dosya yolu belirtilmedi")
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return "", fmt.Errorf("dosya okunamadı: %w", err)
			}
			return string(content), nil
		},
	},
	"write_file": {
		Name:        "write_file",
		Description: "Belirtilen bir dosyaya içerik yazar. DİKKAT: Dosyanın üzerine tamamen yazar. Parametreler: { \"file_path\": \"string\", \"content\": \"string\" }",
		Execute: func(params map[string]string) (string, error) {
			path := params["file_path"]
			content := params["content"]
			if path == "" {
				return "", fmt.Errorf("dosya yolu belirtilmedi")
			}
			err := os.WriteFile(path, []byte(content), 0644)
			if err != nil {
				return "", fmt.Errorf("dosyaya yazılamadı: %w", err)
			}
			return fmt.Sprintf("'%s' dosyasına başarıyla yazıldı.", path), nil
		},
	},
	"append_file": {
		Name:        "append_file",
		Description: "Belirtilen bir dosyanın sonuna içerik ekler. Parametreler: { \"file_path\": \"string\", \"content\": \"string\" }",
		Execute: func(params map[string]string) (string, error) {
			path := params["file_path"]
			content := params["content"]
			if path == "" {
				return "", fmt.Errorf("dosya yolu belirtilmedi")
			}
			f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return "", fmt.Errorf("dosya açılamadı: %w", err)
			}
			defer f.Close()
			if _, err := f.WriteString(content); err != nil {
				return "", fmt.Errorf("dosyaya ekleme yapılamadı: %w", err)
			}
			return fmt.Sprintf("'%s' dosyasına başarıyla ekleme yapıldı.", path), nil
		},
	},
	"run_shell_command": {
		Name:        "run_shell_command",
		Description: "Bir terminal komutunu (shell command) çalıştırır. Parametreler: { \"command\": \"string\" }",
		Execute: func(params map[string]string) (string, error) {
			command := params["command"]
			if command == "" {
				return "", fmt.Errorf("komut belirtilmedi")
			}
			out, err := exec.Command("bash", "-c", command).CombinedOutput()
			if err != nil {
				return "", fmt.Errorf("komut çalıştırılamadı: %s, hata: %w", string(out), err)
			}
			return string(out), nil
		},
	},
}

// GenerateToolsPrompt, AI'ya sunulacak olan araçların dinamik ve sade bir kılavuzunu oluşturur.
func GenerateToolsPrompt() string {
	var promptBuilder strings.Builder
	promptBuilder.WriteString("# KULLANABİLECEĞİN ARAÇLAR\n\n")
	promptBuilder.WriteString("Aşağıda, kullanıcı isteklerini yerine getirmek için kullanabileceğin araçların bir listesi bulunmaktadır:\n\n")

	for _, tool := range ToolRegistry {
		promptBuilder.WriteString(fmt.Sprintf("## Araç: %s\n", tool.Name))
		promptBuilder.WriteString(fmt.Sprintf("- Açıklama: %s\n\n", tool.Description))
	}

	return promptBuilder.String()
}
