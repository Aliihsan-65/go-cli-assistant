package main

import (
	"bufio"
	"bytes"
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

// handleToolCall, bir araç çağrısını yönetir, çalıştırır, geçmişi kaydeder ve başarı durumunu döndürür.
func handleToolCall(toolCall ollama.ToolCall, conversationHistory *string, rawJSON string) bool {
	if toolCall.ToolName == "" {
		pterm.Error.Println("AI, bir araç çağırmaya çalıştı ancak hangisi olduğunu belirtmedi.")
		*conversationHistory += "Asistan: [Hata: İsimsiz bir araç çağrı isteği alındı.]\n"
		return false
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
		return false
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
			return false
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
		return false
	}

	result, err := tool.Execute(toolCall.Params)

	if err != nil {
		pterm.Error.Println("Araç hatası:", err)
		turnHistory += fmt.Sprintf("Araç-Sonucu: [Hata: %v]\n", err)
		*conversationHistory += turnHistory
		return false
	}

	pterm.DefaultBox.WithTitle("Araç Çıktısı: " + tool.Name).Println(result)
	turnHistory += "Araç-Sonucu: " + result + "\n"
	*conversationHistory += turnHistory
	return true
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
Cevabın SADECE ve HER ZAMAN aşağıdaki gibi iç içe geçmiş yapıda olmalı:
{"type":"tool_call","tool_call":{"tool_name":"ARAÇ_ADI","params":{"PARAMETRE_ADI":"DEĞER"}}}

# SIK YAPILAN HATA (BUNU YAPMA!)
Aşağıdaki gibi düz bir yapı KULLANMA. Bu YANLIŞTIR ve programın çökmesine neden olur:
` + "```json" + `
// YANLIŞ ÖRNEK - DÜZ YAPI
{
  "type": "tool_call",
  "tool_name": "run_shell_command",
  "params": {}
}
` + "```" + `

# DOĞRU YAPI
` + "`tool_name`" + ` ve ` + "`params`" + ` alanları, her zaman ` + "`tool_call`" + ` adlı bir anahtarın İÇİNDE olmalıdır. Örnek:
` + "```json" + `
// DOĞRU ÖRNEK - İÇ İÇE YAPI
{
  "type": "tool_call",
  "tool_call": {
    "tool_name": "run_shell_command",
    "params": {
      "command": "ls -l"
    }
  }
}
` + "```" + `

# ÖRNEK
Kullanıcı İsteği: "bu dizini listele"
SENİN CEVABIN: {"type":"tool_call","tool_call":{"tool_name":"list_directory","params":{"file_path":"."}}}`

		toolsListPrompt := tools.GenerateToolsPrompt()
		baseSystemPrompt := fmt.Sprintf("%s\n\n%s", toolPrompt, toolsListPrompt)

		var inTrainingMode = false
		var lastRequestForTraining = ""
		var lastSuccessfulJSONForTraining = ""

		for {
			promptPrefix := pterm.LightYellow("Siz (Araç Modu): ")
			if inTrainingMode {
				promptPrefix = pterm.LightMagenta("Siz (Eğitim Modu): ")
			}
			pterm.DefaultBasicText.Print(promptPrefix)
			userInput, _ := reader.ReadString('\n')
			userInput = strings.TrimSpace(userInput)

			if strings.ToLower(userInput) == "exit" || strings.ToLower(userInput) == "çıkış" {
				break
			}

			if userInput == "/eğit" {
				if !inTrainingMode {
					inTrainingMode = true
					pterm.Info.Println("Eğitim modu başlatıldı. Lütfen öğretmek istediğiniz komutu girin.")
					lastRequestForTraining = ""
					lastSuccessfulJSONForTraining = ""
					continue
				} else {
					if lastRequestForTraining != "" && lastSuccessfulJSONForTraining != "" {
						pterm.Info.Println("Eğitim modu sonlandırılıyor. Ders hafızaya kaydediliyor...")
						embedding, err := ollama.GenerateEmbedding(cfg.Ollama.URL, cfg.Ollama.EmbeddingModel, lastRequestForTraining)
						if err != nil {
							pterm.Warning.Printf("Hafızaya kaydetmek için embedding oluşturulamadı: %v\n", err)
						} else {
							uuid := uuid.New().String()
							err = chromaClient.Add(uuid, embedding, lastRequestForTraining, lastSuccessfulJSONForTraining)
							if err != nil {
								pterm.Warning.Printf("Ders hafızaya eklenemedi: %v\n", err)
							} else {
								pterm.Success.Println("Yeni ders hafızaya başarıyla eklendi!")
							}
						}
					} else {
						pterm.Warning.Println("Kaydedilecek başarılı bir komut bulunamadı. Eğitim modu iptal edildi.")
					}
					inTrainingMode = false
					lastRequestForTraining = ""
					lastSuccessfulJSONForTraining = ""
					continue
				}
			}

			if userInput == "/showmemory" {
				pterm.Info.Println("Hafızadaki tüm dersler getiriliyor...")
				examples, err := chromaClient.GetAllExamples()
				if err != nil {
					pterm.Error.Printf("Hafıza alınamadı: %v\n", err)
					continue
				}

				if len(examples) == 0 {
					pterm.Info.Println("Hafıza boş.")
					continue
				}

				pterm.Info.Printf("%d adet ders bulundu:\n", len(examples))
				for i, ex := range examples {
					boxTitle := fmt.Sprintf("Ders #%d", i+1)
					userRequest := ex["user_request"]
					toolCallJSON := ex["tool_call_json"]

					var prettyJSON bytes.Buffer
					if err := json.Indent(&prettyJSON, []byte(toolCallJSON), "", "  "); err != nil {
						prettyJSON.WriteString(toolCallJSON)
					}

					content := fmt.Sprintf("Kullanıcı İsteği: %s\nDoğru Komut:\n%s", userRequest, prettyJSON.String())
					pterm.DefaultBox.WithTitle(boxTitle).Println(content)
				}
				continue
			}

			// Normal veya Eğitim modunda komut işleme
			inputEmbedding, err := ollama.GenerateEmbedding(cfg.Ollama.URL, cfg.Ollama.EmbeddingModel, userInput)
			var examplesText string
			if err != nil {
				pterm.Warning.Println("Girdi için embedding oluşturulamadı, hafıza sorgulanamıyor.")
			} else {
				examples, distances, err := chromaClient.QueryExamples(inputEmbedding, 2)
				if err != nil {
					pterm.Warning.Printf("Hafıza sorgulanırken hata oluştu: %v\n", err)
				} else if len(examples) > 0 {
					examplesText = formatExamples(examples)
					pterm.Info.Printf("%d adet benzer örnek hafızadan bulundu (mesafeler: %.4f) ve prompt'a eklendi.\n", len(examples), distances)
				}
			}

			cwd, err := os.Getwd()
			if err != nil {
				pterm.Warning.Printf("Mevcut çalışma dizini alınamadı: %v\n", err)
				cwd = "(bilinmiyor)"
			}

			conversationHistory += "Kullanıcı: " + userInput + "\n"
			finalPrompt := fmt.Sprintf(`%s\n%s\n\n# MEVCUT ÇALIŞMA DİZİNİ\n%s\n\n--- Önceki Konuşma ---\n%s\n---------------------\n\nKullanıcı İsteği: %s`, baseSystemPrompt, examplesText, cwd, conversationHistory, userInput)

			spinner, _ := pterm.DefaultSpinner.Start("Uzman AI düşünüyor...")
			responseStr, err := ollama.Generate(cfg.Ollama.URL, cfg.Ollama.Model, finalPrompt)
			spinner.Stop()
			if err != nil {
				pterm.Error.Printf("Ollama'dan cevap alınamadı: %v", err)
				continue
			}

			jsonStr := extractJSONObject(responseStr)
			if jsonStr == "" {
				pterm.Warning.Println("AI'nın cevabında geçerli bir JSON bloğu bulunamadı. Ham cevap:")
				pterm.Println(responseStr)
				conversationHistory += "Asistan: [Hata: JSON bulunamadı] " + responseStr + "\n"
				if inTrainingMode {
					pterm.Warning.Println("Eğitim modunda komut başarısız oldu. Moddan çıkılıyor.")
					inTrainingMode = false
				}
				continue
			}

			var aiResponse ollama.AIResponse
			err = json.Unmarshal([]byte(jsonStr), &aiResponse)
			if err != nil {
				pterm.Warning.Printf("AI geçerli bir JSON formatında cevap vermedi. Hata: %v\nHam Cevap:", err)
				pterm.Println(responseStr)
				conversationHistory += "Asistan: [Hata: Geçersiz JSON] " + responseStr + "\n"
				if inTrainingMode {
					pterm.Warning.Println("Eğitim modunda komut başarısız oldu. Moddan çıkılıyor.")
					inTrainingMode = false
				}
				continue
			}

			if aiResponse.ToolCall.ToolName != "" {
				isSuccess := handleToolCall(aiResponse.ToolCall, &conversationHistory, jsonStr)
				if inTrainingMode {
					if isSuccess {
						lastRequestForTraining = userInput
						lastSuccessfulJSONForTraining = jsonStr
						pterm.Success.Println("Eğitim komutu başarıyla çalıştı. Kaydetmek için tekrar /eğit komutunu girin.")
					} else {
						pterm.Warning.Println("Eğitim sırasında komut başarısız oldu. Moddan çıkılıyor.")
						inTrainingMode = false
					}
				}
			} else {
				pterm.Error.Println("AI geçerli bir araç çağrısı döndürmedi. Yanıt Tipi:", aiResponse.Type)
				pterm.Warning.Println("Alınan JSON:", jsonStr)
				conversationHistory += "Asistan: [Hata: Geçersiz araç çağrısı] " + jsonStr + "\n"
				if inTrainingMode {
					pterm.Warning.Println("Eğitim modunda komut başarısız oldu. Moddan çıkılıyor.")
					inTrainingMode = false
				}
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

			cwd, err := os.Getwd()
			if err != nil {
				pterm.Warning.Printf("Mevcut çalışma dizini alınamadı: %v\n", err)
				cwd = "(bilinmiyor)"
			}

			conversationHistory += "Kullanıcı: " + userInput + "\n"
			finalPrompt := fmt.Sprintf(`%s\n\n# MEVCUT ÇALIŞMA DİZİNİ\n%s\n\n--- Önceki Konuşma ---\n%s\n---------------------\n\nKullanıcı İsteği: %s`, chatPrompt, cwd, conversationHistory, userInput)

			spinner, _ := pterm.DefaultSpinner.Start("AI düşünüyor...")
			responseStr, err := ollama.Generate(cfg.Ollama.URL, cfg.Ollama.Model, finalPrompt)
			spinner.Stop()
			if err != nil {
				pterm.Error.Printf("Ollama'dan cevap alınamadı: %v", err)
				continue
			}

			cleanResponse := strings.TrimSpace(responseStr)
			pterm.DefaultBasicText.Println(pterm.LightGreen(cleanResponse))
			conversationHistory += "Asistan: " + cleanResponse + "\n"
		}
	}
}