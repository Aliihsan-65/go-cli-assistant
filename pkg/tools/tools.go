package tools

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Tool, AI tarafından kullanılabilecek bir aracı temsil eder.
type Tool struct {
	Name        string
	Description string
	// Params, aracın alabileceği parametreleri ve açıklamalarını tutar.
	Params      map[string]string
	// Execute, aracın asıl işlevini yerine getiren fonksiyondur.
	Execute     func(params map[string]string) (string, error)
}

// ToolRegistry, sistemdeki tüm araçları tutar.
var ToolRegistry = make(map[string]Tool)

// init, program başlarken araçları kaydeder.
func init() {
	RegisterTool(Tool{
		Name: "run_shell_command",
		Description: `Kullanıcı bir terminal komutu çalıştırmak, bir programı yürütmek veya bir paket kurmak (örneğin pacman ile) istediğinde kullanılır. Bu çok güçlü bir araçtır. İnteraktif onay isteyen komutlarda takılmamak için '--noconfirm', '-y' gibi bayrakları kullanmaya çalış. ASLA kullanıcı onayı olmadan 'sudo' ile komut çalıştırmayı deneme.`,
		Params:      map[string]string{"command": "çalıştırılacak tam terminal komutu"},
		Execute: func(params map[string]string) (string, error) {
			command := params["command"]
			if command == "" {
				return "", fmt.Errorf("çalıştırılacak komut belirtilmedi")
			}
			// Komutu ve argümanlarını ayır
			parts := strings.Fields(command)
			cmd := exec.Command(parts[0], parts[1:]...)

			// Komutun çıktısını (stdout ve stderr) yakala
			output, err := cmd.CombinedOutput()
			if err != nil {
				return "", fmt.Errorf("komut çalıştırılırken hata oluştu: %w, Çıktı: %s", err, string(output))
			}
			return fmt.Sprintf("Komut başarıyla çalıştırıldı. Çıktı:\n%s", string(output)), nil
		},
	})

	RegisterTool(Tool{
		Name:        "get_current_time",
		Description: "Kullanıcı saati, tarihi veya zamanı sorduğunda mevcut sistem saatini ve tarihini almak için kullanılır.",
		Params:      map[string]string{},
		Execute: func(params map[string]string) (string, error) {
			currentTime := time.Now().Format("2006-01-02 15:04:05")
			return fmt.Sprintf("Mevcut sistem zamanı: %s", currentTime), nil
		},
	})

	RegisterTool(Tool{
		Name:        "list_directory",
		Description: "Kullanıcı bir dizindeki dosyaları veya klasörleri görmek istediğinde kullanılır. Kullanıcı 'bu dizin', 'mevcut dizin' gibi bir ifade kullanırsa veya bir dizin belirtmezse, path parametresi için '.' kullanmalısın.",
		Params:      map[string]string{"path": "listelenecek dizinin yolu"},
		Execute: func(params map[string]string) (string, error) {
			path := params["path"]
			if path == "" {
				path = "."
			}
			entries, err := os.ReadDir(path)
			if err != nil {
				return "", fmt.Errorf("dizin '%s' okunurken hata oluştu: %w", path, err)
			}
			var fileNames []string
			for _, e := range entries {
				fileNames = append(fileNames, e.Name())
			}
			return "Dizin içeriği:\n" + strings.Join(fileNames, "\n"), nil
		},
	})

	RegisterTool(Tool{
		Name:        "read_file",
		Description: "Kullanıcı belirli bir dosyanın içeriğini okumak veya görmek istediğinde kullanılır.",
		Params:      map[string]string{"path": "okunacak dosyanın yolu"},
		Execute: func(params map[string]string) (string, error) {
			path := params["path"]
			if path == "" {
				return "", fmt.Errorf("dosya yolu belirtilmedi")
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return "", fmt.Errorf("dosya '%s' okunurken hata oluştu: %w", path, err)
			}
			return "Dosya içeriği:\n" + string(content), nil
		},
	})

	RegisterTool(Tool{
		Name:        "write_file",
		Description: "Kullanıcı bir dosyaya bir şey yazmak, bir dosyayı değiştirmek veya yeni bir dosya oluşturmak istediğinde kullanılır.",
		Params:      map[string]string{"path": "yazılacak dosyanın yolu", "content": "dosyaya yazılacak içerik"},
		Execute: func(params map[string]string) (string, error) {
			path := params["path"]
			content := params["content"]
			if path == "" {
				return "", fmt.Errorf("dosya yolu belirtilmedi")
			}
			err := os.WriteFile(path, []byte(content), 0644)
			if err != nil {
				return "", fmt.Errorf("'%s' dosyasına yazılırken hata oluştu: %w", path, err)
			}
			return fmt.Sprintf("'%s' dosyası başarıyla yazıldı.", path), nil
		},
	})
}

// RegisterTool, yeni bir aracı kayda ekler.
func RegisterTool(tool Tool) {
	ToolRegistry[tool.Name] = tool
}

// GenerateToolsPrompt, kayıtlı tüm araçlardan LLM için bir sistem prompt'u oluşturur.
func GenerateToolsPrompt() string {
	var promptBuilder strings.Builder
	promptBuilder.WriteString("Kullanabileceğin Araçlar:\n")
	for _, tool := range ToolRegistry {
		promptBuilder.WriteString(fmt.Sprintf(`- tool_name: "%s"
`, tool.Name))
		promptBuilder.WriteString(fmt.Sprintf(`  - description: "%s"
`, tool.Description))

		var paramParts []string
		for pName, pDesc := range tool.Params {
			paramParts = append(paramParts, fmt.Sprintf(`"%s": "%s"`, pName, pDesc))
		}
		promptBuilder.WriteString(fmt.Sprintf(`  - params: {%s}
`, strings.Join(paramParts, ", ")))
	}
	return promptBuilder.String()
}