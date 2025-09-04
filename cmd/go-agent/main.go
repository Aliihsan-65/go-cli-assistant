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

	"github.com/google/uuid"
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
		msg = pterm.Warning.Sprintf("DİKKAT: '%s' aracını çalıştırmak '%s' dosyasının üzerine yazabilir/değiştirebilir. Onaylıyor musunuz?", tool.Name, toolCall.Params["file_path"])
	case "append_file":
		msg = pterm.Warning.Sprintf("DİKKAT: '%s' aracını çalıştırmak '%s' dosyasına ekleme yapacak. Onaylıyor musunuz?", tool.Name, toolCall.Params["file_path"])
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

// formatExamples, veritabanından gelen örnekleri LLM'in anlayacağı bir formata dönüştürür.
func formatExamples(examples []map[string]string) string {
	if len(examples) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("\n# BAŞARILI ÖRNEKLER\n")
	builder.WriteString("# Geçmişte doğru olarak çözülmüş bu örneklerden öğrenerek yeni görevi tamamla.\n")

	for i, ex := range examples {
		builder.WriteString(fmt.Sprintf("# Örnek %d:\n", i+1))
		builder.WriteString(fmt.Sprintf("#   Kullanıcı İsteği: \"%s\"\n", ex["user_request"]))
		builder.WriteString(fmt.Sprintf("#   Üretilen Doğru Komut: %s\n", ex["tool_call_json"]))
	}
	return builder.String()
}

func main() {
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		pterm.Fatal.Printf("Yapılandırma yüklenemedi: %v\n", err)
	}

	chromaClient := memory.NewChromaClient(cfg.Chroma.URL, cfg.Chroma.CollectionName)

	// --- Test için Manuel Örnek Ekleme ---
	pterm.Info.Println("Test için hafızaya manuel bir ders ekleniyor...")
	goModReadRequest := "go.mod dosyasının içeriğini oku"
	goModReadToolCall := `{"type":"tool_call","tool_call":{"tool_name":"read_file","params":{"path":"go.mod"}}}`
	embedding, err := ollama.GenerateEmbedding(cfg.Ollama.URL, cfg.Ollama.EmbeddingModel, goModReadRequest)
	if err != nil {
		pterm.Warning.Printf("Test örneği için embedding oluşturulamadı: %v\n", err)
	} else {
		uuid := uuid.New().String()
		err = chromaClient.Add(uuid, embedding, goModReadRequest, goModReadToolCall)
		if err != nil {
			pterm.Warning.Printf("Test örneği hafızaya eklenemedi: %v\n", err)
		} else {
			pterm.Success.Println("Test dersi hafızaya başarıyla eklendi.")
		}
	}
	// ---------------------------------------

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
		toolPrompt := `SEN, bir uzman asistansın. Görevin, kullanıcı isteğini analiz edip, cevabını **İSTİSNASIZ OLARAK** aşağıda belirtilen JSON formatında vermektir. Başka HİÇBİR format, metin veya açıklama kullanma.

# ZORUNLU ÇIKTI FORMATI
Cevabın SADECE şu JSON yapısında olmalı:
{"type":"tool_call","tool_call":{"tool_name":"ARAÇ_ADI","params":{"PARAMETRE_ADI":"DEĞER"}}}

# KURALLAR
1. type alanı her zaman "tool_call" olmalıdır.
2. tool_name alanı, sana verilen listedeki araçlardan biri olmalıdır (örneğin: run_shell_command).
3. params nesnesi, o aracın gerektirdiği parametreleri içermelidir.

# ÖRNEK
Kullanıcı İsteği: "bu dizini listele"
SENİN CEVABIN: {"type":"tool_call","tool_call":{"tool_name":"list_directory","params":{"path":". "}}}
`

		toolsListPrompt := tools.GenerateToolsPrompt()
		baseSystemPrompt := fmt.Sprintf("%s\n\n%s", toolPrompt, toolsListPrompt)

		for {
			pterm.DefaultBasicText.Print(pterm.LightYellow("Siz (Araç Modu): "))
			userInput, _ := reader.ReadString('\n')
			userInput = strings.TrimSpace(userInput)

			if strings.ToLower(userInput) == "exit" || strings.ToLower(userInput) == "çıkış" {
				break
			}

			// Hafızadan ilgili örnekleri bul
			inputEmbedding, err := ollama.GenerateEmbedding(cfg.Ollama.URL, cfg.Ollama.EmbeddingModel, userInput)
			var examplesText string
			if err != nil {
				pterm.Warning.Println("Girdi için embedding oluşturulamadı, hafıza sorgulanamıyor.")
			} else {
				examples, err := chromaClient.QueryExamples(inputEmbedding, 2, 1.5) // 2 örnek al, eşik değeri 1.5 (deneme)
				if err != nil {
					pterm.Warning.Printf("Hafıza sorgulanırken hata oluştu: %v\n", err)
				} else if len(examples) > 0 {
					examplesText = formatExamples(examples)
					pterm.Info.Printf("%d adet benzer örnek hafızadan bulundu ve prompt'a eklendi.\n", len(examples))
				}
			}

			// Mevcut çalışma dizinini al
			cwd, err := os.Getwd()
			if err != nil {
				pterm.Warning.Printf("Mevcut çalışma dizini alınamadı: %v\n", err)
				cwd = "(bilinmiyor)"
			}

			conversationHistory += "Kullanıcı: " + userInput + "\n"
			finalPrompt := fmt.Sprintf("%s\n%s\n\n# MEVCUT ÇALIŞMA DİZİNİ\n%s\n\n--- Önceki Konuşma ---\n%s\n---------------------\n\nKullanıcı İsteği: %s", baseSystemPrompt, examplesText, cwd, conversationHistory, userInput)

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

			if aiResponse.ToolCall.ToolName != "" {
				// The AI provided a tool_call object, which is what we want in Tool Mode.
				handleToolCall(aiResponse.ToolCall, &conversationHistory, jsonStr)
			} else {
				// The JSON was valid, but it wasn't a tool_call or was missing a tool_name.
				pterm.Error.Println("AI geçerli bir araç çağrısı döndürmedi. Yanıt Tipi:", aiResponse.Type)
				pterm.Warning.Println("Alınan JSON:", jsonStr) // Hata ayıklama için eklendi
				conversationHistory += "Asistan: [Hata: Geçersiz araç çağrısı] " + jsonStr + "\n"
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

			// Mevcut çalışma dizinini al (sohbet modunda da bağlam için faydalı olabilir)
			cwd, err := os.Getwd()
			if err != nil {
				pterm.Warning.Printf("Mevcut çalışma dizini alınamadı: %v\n", err)
				cwd = "(bilinmiyor)"
			}

			conversationHistory += "Kullanıcı: " + userInput + "\n"
			finalPrompt := fmt.Sprintf("%s\n\n# MEVCUT ÇALIŞMA DİZİNİ\n%s\n\n--- Önceki Konuşma ---\n%s\n---------------------\n\nKullanıcı İsteği: %s", chatPrompt, cwd, conversationHistory, userInput)

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
