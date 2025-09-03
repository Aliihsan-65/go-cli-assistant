package main

import (
	"encoding/json"
	"fmt"
	"go-agent/pkg/config"
	"go-agent/pkg/memory"
	"go-agent/pkg/ollama"
	"go-agent/pkg/tools"
	"strings"

	"github.com/pterm/pterm"
)

// extractTagContent extracts the content between specified XML tags from a text.
func extractTagContent(text, tagName string) string {
	startTag := "<" + tagName + ">"
	endTag := "</" + tagName + ">"

	startIndex := strings.Index(text, startTag)
	if startIndex == -1 {
		return ""
	}

	startIndex += len(startTag)
	endIndex := strings.Index(text[startIndex:], endTag)
	if endIndex == -1 {
		return ""
	}

	return strings.TrimSpace(text[startIndex : startIndex+endIndex])
}

// handleToolCall manages a tool call, executes it, and records the history.
func handleToolCall(toolCall ollama.ToolCall, conversationHistory *string, rawJSON string, dusunce string) {
	if toolCall.ToolName == "" {
		pterm.Error.Println("AI attempted to call a tool but did not specify which one.")
		*conversationHistory += "Asistan: [Hata: İsimsiz bir araç çağrı isteği alındı.]\n"
		return
	}

	if toolCall.ToolName == "run_command" {
		pterm.Warning.Println("AI attempted to call 'run_command', correcting to 'run_shell_command'.")
		toolCall.ToolName = "run_shell_command"
	}

	pterm.Info.Println("LLM intends to use tool:", toolCall.ToolName)
	pterm.Info.Println("Parameters:", toolCall.Params)

	tool, exists := tools.ToolRegistry[toolCall.ToolName]
	if !exists {
		pterm.Error.Println("Unknown tool requested:", toolCall.ToolName)
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
			pterm.Error.Println("Missing 'command' parameter for run_shell_command.")
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

	var turnHistory string
	if dusunce != "" {
		turnHistory += "Asistan: <dusunce>" + dusunce + "</dusunce>\n"
	}
	turnHistory += "<arac_cagrisi>" + rawJSON + "</arac_cagrisi>\n"

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
	// TODO: Add successful expert responses to memory here.
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
	pterm.Info.Println("Hafıza (ChromaDB) istemcisi başarıyla başlatıldı.")
	pterm.Println()

	baseSystemPrompt := `SEN, bir Go programı tarafından kullanılan ve araçları kullanabilen bir yapay zeka asistanısın.

# KESİN VE NET KURALLAR:

1.  **CEVAP VERME STRATEJİN:**
    A. **DOĞRUDAN CEVAP VER:** Eğer kullanıcı selam veriyor, sohbet ediyor, hal hatır soruyor VEYA sorduğu soru ("bisikletçi beslenmesi nedir" gibi) genel bilgi, ansiklopedik bilgi veya tanım içeriyorsa, **KESİNLİKLE ARAÇ KULLANMA**. Bu durumlarda, soruyu doğrudan kendi bilgilerinle, düz metin olarak cevapla.
    B. **ARAÇ KULLAN:** Eğer kullanıcı senden dosya sistemi üzerinde bir işlem yapmanı (oku, yaz), bir komut çalıştırmanı veya internetten **GÜNCEL, ANLIK veya ÇOK SPESİFİK** bir bilgi (örneğin "bugünkü hava durumu", "X şirketinin hisse senedi fiyatı") bulmanı istiyorsa, o zaman araç kullan.

2.  **ARAÇ KULLANIM FORMATI:** Araç kullanmaya karar verirsen, **SADECE** aşağıdaki formatı kullan. Başka HİÇBİR ŞEY yazma.
    - Önce "<dusunce>..." etiketi içine kısa bir plan yaz.
    - SONRA, "<arac_cagrisi>..." etiketi içine JSON olarak aracı çağır.

    ` + "```xml" + `
    <dusunce>Kullanıcının isteğini yerine getirmek için X aracını kullanacağım.</dusunce>
    <arac_cagrisi>{"type":"tool_call","tool_call":{"tool_name":"araç_adı","params":{"parametre":"değer"}}}</arac_cagrisi>
    ` + "```" + `

# UNUTMA:
- ASLA kendi kendine "Araç-Sonucu:" diye bir çıktı üretme. Sistem bunu sana sağlayacak.
- ASLA kullanıcıya soru sorma veya onay isteme. Sadece kuralları uygula.`

	toolsPrompt := tools.GenerateToolsPrompt()
	fullSystemPrompt := fmt.Sprintf("%s\n\n%s", baseSystemPrompt, toolsPrompt)

	for {
		userInput, _ := pterm.DefaultInteractiveTextInput.Show("Siz")

		if strings.ToLower(userInput) == "exit" || strings.ToLower(userInput) == "çıkış" {
			pterm.Info.Println("Görüşmek üzere!")
			break
		}

		spinner, _ := pterm.DefaultSpinner.Start("Hafıza kontrol ediliyor...")
		embedding, err := ollama.GenerateEmbedding(cfg.Ollama.URL, cfg.Ollama.EmbeddingModel, userInput)

		var finalResponseStr string

		if err != nil || embedding == nil || len(embedding) == 0 {
			spinner.Fail(fmt.Sprintf("Embedding oluşturulamadı veya boş: %v", err))
			spinner.UpdateText("Embedding hatası, doğrudan yapay zeka düşünüyor...")
		} else {
			cachedResult, err := chromaClient.Query(embedding, 1, cfg.Chroma.SimilarityThreshold)
			if err != nil {
				spinner.Fail(fmt.Sprintf("Hafıza sorgulanamadı: %v", err))
				spinner.UpdateText("Hafıza sorgusu başarısız, yapay zeka düşünüyor...")
			} else if cachedResult != "" {
				spinner.Success("Cevap hafızadan bulundu!")
				pterm.DefaultBox.WithTitle("Hafızadan Gelen Cevap").Println(pterm.LightGreen(cachedResult))
				finalResponseStr = cachedResult // Use the response from memory
			} else {
				spinner.UpdateText("Hafızada bir şey bulunamadı, yapay zeka düşünüyor...")
			}
		}

		// If no response from memory or memory query failed, ask the LLM
		if finalResponseStr == "" {
			conversationHistory += "Kullanıcı: " + userInput + "\n"

			finalPrompt := fmt.Sprintf("%s\n\n--- Önceki Konuşma ---\n%s\n---------------------\n\nKullanıcı İsteği: %s", fullSystemPrompt, conversationHistory, userInput)

			responseStr, err := ollama.Generate(cfg.Ollama.URL, cfg.Ollama.Model, finalPrompt)
			if err != nil {
				spinner.Fail(fmt.Sprintf("Ollama'dan cevap alınamadı: %v", err))
				continue
			}
			spinner.Success("Cevap alındı!")
			finalResponseStr = responseStr // Use the response from LLM
		}

		// Process the final response (from memory or LLM)
		dusunce := extractTagContent(finalResponseStr, "dusunce")
		aracCagrisiJSON := extractTagContent(finalResponseStr, "arac_cagrisi")

		if aracCagrisiJSON == "" && strings.HasPrefix(strings.TrimSpace(finalResponseStr), "{") {
			pterm.Warning.Println("AI, '<arac_cagrisi>' etiketini kullanmayı unuttu, ancak bir JSON nesnesi döndürdü. Yine de işlenmeye çalışılıyor.")
			aracCagrisiJSON = finalResponseStr
		}

		if aracCagrisiJSON != "" {
			if dusunce != "" {
				pterm.DefaultBox.WithTitle("AI Düşünce Süreci").WithTextStyle(pterm.NewStyle(pterm.FgLightMagenta)).Println(dusunce)
			}

			trimmedJSON := strings.TrimSpace(aracCagrisiJSON)
			if strings.HasPrefix(trimmedJSON, "{") && strings.HasSuffix(trimmedJSON, "}") {
				var aiResponse ollama.AIResponse
				err = json.Unmarshal([]byte(trimmedJSON), &aiResponse)
				if err != nil {
					pterm.Error.Printf("Araç çağrısı JSON formatında değil veya hatalı: %v\nGelen JSON: %s\n", err, trimmedJSON)
					conversationHistory += "Asistan: [Hata: Geçersiz formatta araç çağrısı alındı.]\n"
					continue
				}

				if aiResponse.Type == "tool_call" {
					handleToolCall(aiResponse.ToolCall, &conversationHistory, trimmedJSON, dusunce)
				} else {
					pterm.Error.Printf("Beklenmedik cevap tipi: '%s'\n", aiResponse.Type)
					conversationHistory += "Asistan: [Hata: Beklenmedik bir cevap tipi döndü.]\n"
				}
			} else {
				// JSON değilse, düz metin olarak yazdır
				pterm.DefaultBasicText.WithStyle(pterm.NewStyle(pterm.FgLightCyan)).Println(aracCagrisiJSON)
				conversationHistory += "Asistan: " + aracCagrisiJSON + "\n"
			}
		} else {
			cleanResponse := strings.TrimSpace(finalResponseStr)
			cleanResponse = strings.TrimPrefix(cleanResponse, "Asistan:")
			cleanResponse = strings.TrimSpace(cleanResponse)

			pterm.DefaultBasicText.WithStyle(pterm.NewStyle(pterm.FgLightCyan)).Println(cleanResponse)
			conversationHistory += "Asistan: " + cleanResponse + "\n"
		}
	}
}
