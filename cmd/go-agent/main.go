package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go-agent/pkg/config"
	"go-agent/pkg/memory"
	"go-agent/pkg/ollama"
	"go-agent/pkg/tools"
	"os"
	"strings"

	"github.com/pterm/pterm"
)

// extractJSONObject, bir metin içerisindeki ilk JSON nesnesini ({} arasındaki) bulup çıkarır.
func extractJSONObject(text string) string {
	startIndex := strings.Index(text, "{")
	if startIndex == -1 {
		return ""
	}
	endIndex := strings.LastIndex(text, "}")
	if endIndex == -1 || endIndex < startIndex {
		return ""
	}
	return text[startIndex : endIndex+1]
}

// handleToolCall, bir araç çağrısını yönetir, çalıştırır ve geçmişi kaydeder.
func handleToolCall(toolCall ollama.ToolCall, conversationHistory *string, rawJSON string) {
	if toolCall.ToolName == "" {
		pterm.Error.Println("AI, bir araç çağırmaya çalıştı ancak hangisi olduğunu belirtmedi.")
		*conversationHistory += "Asistan: [Hata: İsimsiz bir araç çağrı isteği alındı.]\n"
		return
	}

	if toolCall.ToolName == "run_command" {
		pterm.Warning.Println("AI, 'run_command' çağırmaya çalıştı, 'run_shell_command' olarak düzeltiliyor.")
		toolCall.ToolName = "run_shell_command"
	}

	pterm.Info.Println("LLM şu aracı kullanmak istiyor:", toolCall.ToolName)
	pterm.Info.Println("Parametreler:", toolCall.Params)

	tool, exists := tools.ToolRegistry[toolCall.ToolName]
	if !exists {
		pterm.Error.Println("Bilinmeyen araç istendi:", toolCall.ToolName)
		*conversationHistory += "Asistan: [Hata: Bilinmeyen bir araç istendi.]\n"
		return
	}

	msg := fmt.Sprintf("'%s' aracını şu parametrelerle çalıştırmayı onaylıyor musunuz: %v", tool.Name, toolCall.Params)

	switch tool.Name {
	case "write_file":
		msg = pterm.Warning.Sprintf("DİKKAT: '%s' aracını çalıştırmak '%s' dosyasının üzerine yazabilir/değiştirebilir. Onaylıyor musunuz?", tool.Name, toolCall.Params["path"])
	case "append_file":
		msg = pterm.Warning.Sprintf("DİKKAT: '%s' aracını çalıştırmak '%s' dosyasına ekleme yapacak. Onaylıyor musunuz?", tool.Name, toolCall.Params["path"])
	case "run_shell_command":
		commandToRun, exists := toolCall.Params["command"]
		if !exists {
			pterm.Error.Println("run_shell_command için 'command' parametresi eksik.")
			*conversationHistory += "Asistan: [Hata: 'command' parametresi eksik.]\n"
			return
		}
		if strings.Contains(commandToRun, "sudo") {
			msg = pterm.Error.Sprintf("!!! AŞIRI TEHLİKELİ İŞLEM !!! '%s' komutunu 'sudo' ile çalıştırmak üzeresiniz. Bu, sisteminizde kalıcı değişiklikler yapabilir veya zarar verebilir. Emin misiniz?", commandToRun)
		} else {
			msg = pterm.Warning.Sprintf("DİKKAT: '%s' komutunu terminalde çalıştırmak üzeresiniz. Onaylıyor musunuz?", commandToRun)
		}
	}

	approved, _ := pterm.DefaultInteractiveConfirm.Show(msg)

	turnHistory := rawJSON + "\n"

	if !approved {
		pterm.Warning.Println("İşlem iptal edildi.")
		turnHistory += "Araç-Sonucu: [Kullanıcı tarafından iptal edildi.]\n"
		*conversationHistory += turnHistory
		return
	}

	result, err := tool.Execute(toolCall.Params)

	if err != nil {
		pterm.Error.Println("Araç hatası:", err)
		turnHistory += fmt.Sprintf("Araç-Sonucu: [Hata: %v]\n", err)
	} else {
		pterm.DefaultBox.WithTitle("Araç Çıktısı: " + tool.Name).Println(result)
		turnHistory += "Araç-Sonucu: " + result + "\n"
	}
	*conversationHistory += turnHistory
}

func main() {
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		pterm.Fatal.Printf("Yapılandırma yüklenemedi: %v\n", err)
	}

	_ = memory.NewChromaClient(cfg.Chroma.URL, cfg.Chroma.CollectionName)

	var conversationHistory string

	pterm.DefaultBigText.WithLetters(pterm.NewLettersFromString("GO AGENT")).Render()
	pterm.Info.Println("Go ile yazılmış, modüler ve yapılandırılabilir AI asistanı.")
	pterm.Println()

	selectedMode, _ := pterm.DefaultInteractiveSelect.WithOptions([]string{
		"Araç Kullanımı (Siber Güvenlik & Pentest Otomasyonu)",
		"Genel Sohbet",
	}).WithDefaultText("Bir çalışma modu seçin").Show()

	pterm.Info.Printf("%s modunda başlatıldı.\n", selectedMode)

	reader := bufio.NewReader(os.Stdin)

	switch selectedMode {
	case "Araç Kullanımı (Siber Güvenlik & Pentest Otomasyonu)":
		toolPrompt := `SEN, bir siber güvenlik ve sızma testi (pentest) uzmanısın. Görevin, kullanıcıdan gelen istekleri analiz edip, cevabını **SADECE ve DOĞRUDAN** aşağıda listelenen araçlardan birini çağıran JSON formatında vermektir.\n\n# KESİN KURALLAR:\n1. **SADECE JSON ÇIKTISI VER:** Cevabın her zaman, istisnasız olarak, zorunlu JSON formatında olmalıdır.\n2. **SADECE GEÇERLİ ARAÇLARI KULLAN:** (tool_name) alanı, sana aşağıda verilen araç listesindeki isimlerden biri olmalıdır. Örneğin, bir komut çalıştırmak için (tool_name) alanına (run_shell_command) yazmalısın. ASLA (nmap) veya (ffuf) gibi bir komut adını (tool_name) olarak kullanma.\n3. **ASLA AÇIKLAMA YAPMA:** JSON dışında hiçbir metin, selamlama, açıklama veya not yazma.\n4. **UZMAN GİBİ KOMUT OLUŞTUR:** Kullanıcının isteğindeki (detaylı, hızlı, sessiz gibi) nyuansları anlayarak komut parametrelerini bir uzman gibi kendin belirle.\n\n# ZORUNLU JSON FORMATI:\n{"type":"tool_call","tool_call":{"tool_name":"GEÇERLİ_BİR_ARAÇ_ADI","params":{"parametre":"değer"}}}`
		toolsListPrompt := tools.GenerateToolsPrompt()
		fullSystemPrompt := fmt.Sprintf("%s\n\n%s", toolPrompt, toolsListPrompt)

		for {
			pterm.DefaultBasicText.Print(pterm.LightYellow("Siz (Araç Modu): "))
			userInput, _ := reader.ReadString('\n')
			userInput = strings.TrimSpace(userInput)

			if strings.ToLower(userInput) == "exit" || strings.ToLower(userInput) == "çıkış" {
				break
			}

			conversationHistory += "Kullanıcı: " + userInput + "\n"
			finalPrompt := fmt.Sprintf("%s\n\n--- Önceki Konuşma ---\n%s\n---------------------\n\nKullanıcı İsteği: %s", fullSystemPrompt, conversationHistory, userInput)

			spinner, _ := pterm.DefaultSpinner.Start("Uzman AI düşünüyor...")
			responseStr, err := ollama.Generate(cfg.Ollama.URL, cfg.Ollama.Model, finalPrompt)
			if err != nil {
				spinner.Fail(fmt.Sprintf("Ollama'dan cevap alınamadı: %v", err))
				continue
			}
			spinner.Success("Cevap alındı!")

			jsonStr := extractJSONObject(responseStr)
			if jsonStr == "" {
				pterm.Warning.Println("AI'nın cevabında geçerli bir JSON bloğu bulunamadı. Ham cevap:")
				pterm.Println(responseStr)
				conversationHistory += "Asistan: [Hata: JSON bulunamadı] " + responseStr + "\n"
				continue
			}

			var aiResponse ollama.AIResponse
			err = json.Unmarshal([]byte(jsonStr), &aiResponse)
			if err != nil {
				pterm.Warning.Printf("AI geçerli bir JSON formatında cevap vermedi. Hata: %v\nHam Cevap:", err)
				pterm.Println(responseStr)
				conversationHistory += "Asistan: [Hata: Geçersiz JSON] " + responseStr + "\n"
				continue
			}

			if aiResponse.Type == "tool_call" {
				handleToolCall(aiResponse.ToolCall, &conversationHistory, jsonStr)
			} else {
				pterm.Error.Printf("Beklenmedik cevap tipi: '%s'\n", aiResponse.Type)
				conversationHistory += "Asistan: [Hata: Beklenmedik cevap tipi] " + jsonStr + "\n"
			}
		}

	case "Genel Sohbet":
		chatPrompt := `SEN, yardımsever bir sohbet asistanısın. Görevin sadece kullanıcıyla sohbet etmektir. Asla ve asla özel formatlar, etiketler veya araçlar kullanma.`

		for {
			pterm.DefaultBasicText.Print(pterm.LightCyan("Siz (Sohbet Modu): "))
			userInput, _ := reader.ReadString('\n')
			userInput = strings.TrimSpace(userInput)

			if strings.ToLower(userInput) == "exit" || strings.ToLower(userInput) == "çıkış" {
				break
			}

			conversationHistory += "Kullanıcı: " + userInput + "\n"
			finalPrompt := fmt.Sprintf("%s\n\n--- Önceki Konuşma ---\n%s\n---------------------\n\nKullanıcı İsteği: %s", chatPrompt, conversationHistory, userInput)

			spinner, _ := pterm.DefaultSpinner.Start("AI düşünüyor...")
			responseStr, err := ollama.Generate(cfg.Ollama.URL, cfg.Ollama.Model, finalPrompt)
			if err != nil {
				spinner.Fail(fmt.Sprintf("Ollama'dan cevap alınamadı: %v", err))
				continue
			}
			spinner.Success("Cevap alındı!")

			cleanResponse := strings.TrimSpace(responseStr)
			pterm.DefaultBasicText.Println(pterm.LightGreen(cleanResponse))
			conversationHistory += "Asistan: " + cleanResponse + "\n"
		}
	}
}
