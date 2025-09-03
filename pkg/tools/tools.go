package tools

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Tool, bir aracı ve onunla ilgili AI'ya yol gösterecek meta verileri tanımlar.
type Tool struct {
	Name        string
	Description string
	// Triggers    []string // Bu aracı düşünmesini tetikleyecek anahtar konular/fiiller
	Examples []string // Tam kullanım örnekleri

	Execute func(params map[string]string) (string, error)
}

// ToolRegistry, sistemdeki tüm araçları zenginleştirilmiş tanımlarıyla birlikte tutar.
var ToolRegistry = map[string]Tool{
	
	"list_directory": {
		Name:        "list_directory",
		Description: "YALNIZCA bir dosya sistemi dizinindeki dosyaları ve klasörleri listelemek için kullanılır. Genel soruları yanıtlamak için KULLANMA.",
		Examples: []string{
			"Kullanıcı: bu dizindeki dosyaları göster -> <arac_cagrisi>{\"type\":\"tool_call\",\"tool_call\":{\"tool_name\":\"list_directory\",\"params\":{\"path\":\".\"}}}<arac_cagrisi>",
			"Kullanıcı: cmd klasöründe ne var? -> <arac_cagrisi>{\"type\":\"tool_call\",\"tool_call\":{\"tool_name\":\"list_directory\",\"params\":{\"path\":\"cmd\"}}}<arac_cagrisi>",
		},
		Execute: func(params map[string]string) (string, error) {
			path := params["path"]
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
		Description: "YALNIZCA yerel dosya sistemindeki belirli bir dosyanın içeriğini okumak için kullanılır.",
		Examples: []string{
			"Kullanıcı: deneme.txt dosyasını oku -> <arac_cagrisi>{\"type\":\"tool_call\",\"tool_call\":{\"tool_name\":\"read_file\",\"params\":{\"path\":\"deneme.txt\"}}}<arac_cagrisi>",
			"Kullanıcı: config.yaml içeriği nedir? -> <arac_cagrisi>{\"type\":\"tool_call\",\"tool_call\":{\"tool_name\":\"read_file\",\"params\":{\"path\":\"config.yaml\"}}}<arac_cagrisi>",
		},
		Execute: func(params map[string]string) (string, error) {
			path := params["path"]
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
		Description: "YALNIZCA yerel dosya sistemindeki belirli bir dosyaya içerik yazar. DİKKAT: Dosyanın üzerine tamamen yazar.",
		Examples: []string{
			"Kullanıcı: yeni.txt dosyasına 'merhaba dünya' yaz -> <arac_cagrisi>{\"type\":\"tool_call\",\"tool_call\":{\"tool_name\":\"write_file\",\"params\":{\"path\":\"yeni.txt\",\"content\":\"merhaba dünya\"}}}<arac_cagrisi>",
		},
		Execute: func(params map[string]string) (string, error) {
			path := params["path"]
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
		Description: "YALNIZCA yerel dosya sistemindeki belirli bir dosyanın sonuna içerik ekler.",
		Examples: []string{
			"Kullanıcı: deneme.txt sonuna '789' ekle -> <arac_cagrisi>{\"type\":\"tool_call\",\"tool_call\":{\"tool_name\":\"append_file\",\"params\":{\"path\":\"deneme.txt\",\"content\":\"789\"}}}<arac_cagrisi>",
		},
		Execute: func(params map[string]string) (string, error) {
			path := params["path"]
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
		Description: "YALNIZCA bir terminal komutunu (shell command) çalıştırmak için kullanılır.",
		Examples: []string{
			"Kullanıcı: go versiyonunu çalıştır -> <arac_cagrisi>{\"type\":\"tool_call\",\"tool_call\":{\"tool_name\":\"run_shell_command\",\"params\":{\"command\":\"go version\"}}}<arac_cagrisi>",
		},
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
	"get_time": {
		Name:        "get_time",
		Description: "Mevcut tarih ve saati döndürür.",
		Examples: []string{
			"Kullanıcı: saat kaç? -> <arac_cagrisi>{\"type\":\"tool_call\",\"tool_call\":{\"tool_name\":\"get_time\",\"params\":{}}}<arac_cagrisi>",
		},
		Execute: func(params map[string]string) (string, error) {
			return time.Now().Format("2006-01-02 15:04:05"), nil
		},
	},
}

// GenerateToolsPrompt, AI'ya sunulacak olan araçların dinamik ve detaylı kullanım kılavuzunu oluşturur.
func GenerateToolsPrompt() string {
	var promptBuilder strings.Builder
	promptBuilder.WriteString("# KULLANABİLECEĞİN ARAÇLAR\n\n")
	promptBuilder.WriteString("Aşağıda, kullanıcı isteklerini yerine getirmek için kullanabileceğin araçların bir listesi bulunmaktadır. Bir aracı KULLANMADAN ÖNCE açıklamasını ve örneklerini DİKKATLİCE oku.\n\n")

	for _, tool := range ToolRegistry {
		promptBuilder.WriteString(fmt.Sprintf("## Araç: %s\n", tool.Name))
		promptBuilder.WriteString(fmt.Sprintf("- Açıklama: %s\n", tool.Description))
		promptBuilder.WriteString("- Örnekler:\n")
		for _, example := range tool.Examples {
			promptBuilder.WriteString(fmt.Sprintf("  - %s\n", example))
		}
		promptBuilder.WriteString("\n")
	}

	return promptBuilder.String()
}
