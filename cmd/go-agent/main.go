package main

import (
	"encoding/json"
	"fmt"
	"go-agent/pkg/config"
	"go-agent/pkg/ollama"
	"go-agent/pkg/tools"
	"strings"

	"github.com/pterm/pterm"
)

// extractTagContent bir metin içinden belirtilen XML etiketinin içeriğini çıkarır.
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

// handleToolCall, bir araç çağrısını yönetir, çalıştırır ve geçmişi kaydeder.
func handleToolCall(toolCall ollama.ToolCall, conversationHistory *string, rawJSON string, dusunce string) {
	if toolCall.ToolName == "" {
		pterm.Error.Println("AI bir araç çağırmak istedi ancak hangi aracı kullanacağını belirtmedi.")
		*conversationHistory += "Asistan: [Hata: İsimsiz bir araç çağırma isteği alındı.]\n"
		return
	}

	// AI'nın "run_command" halüsinasyonunu "run_shell_command" olarak düzelt
	if toolCall.ToolName == "run_command" {
		pterm.Warning.Println("AI 'run_command' aracını çağırmayı denedi, 'run_shell_command' olarak düzeltiliyor.")
		toolCall.ToolName = "run_shell_command"
	}

	pterm.Info.Println("LLM bir araç kullanmak istiyor:", toolCall.ToolName)
	pterm.Info.Println("Parametreler:", toolCall.Params)

	tool, exists := tools.ToolRegistry[toolCall.ToolName]
	if !exists {
		pterm.Error.Println("Bilinmeyen araç isteği:", toolCall.ToolName)
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
		commandToRun := toolCall.Params["command"]
		if strings.Contains(commandToRun, "sudo") {
			msg = pterm.Error.Sprintf("!!! AŞIRI TEHLİKELİ İŞLEM !!! '%s' komutunu 'sudo' ile çalıştırmak üzeresiniz. Bu, sisteminizde kalıcı değişiklikler yapabilir veya zarar verebilir. Emin misiniz?", commandToRun)
		} else {
			msg = pterm.Warning.Sprintf("DİKKAT: '%s' komutunu terminalde çalıştırmak üzeresiniz. Onaylıyor musunuz?", commandToRun)
		}
	}

	approved, _ := pterm.DefaultInteractiveConfirm.Show(msg)

	var turnHistory string
	// Her durumda düşünce ve araç çağrısını geçmişe ekle
	if dusunce != "" {
		turnHistory += "Asistan: <dusunce>" + dusunce + "</dusunce>\n"
	}
	// AI'nın unuttuğu etiketleri yeniden oluşturarak geçmişi daha tutarlı hale getir
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
}

func main() {
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		pterm.Fatal.Printf("Yapılandırma yüklenemedi: %v\n", err)
	}

	var conversationHistory string

	pterm.DefaultBigText.WithLetters(pterm.NewLettersFromString("GO AGENT")).Render()
	pterm.Info.Println("Go ile yazılmış, modüler ve yapılandırılabilir AI asistanı.")
	pterm.Println()

	baseSystemPrompt := `Sen, bir Go programı tarafından kullanılan bir yapay zeka asistanısın.
# GÖREVİN
Kullanıcı isteklerini analiz et ve görevleri adım adım tamamla. Her adımda bir araç kullanabilir veya kullanıcıya cevap verebilirsin.
# CEVAP KURALLARI
1.  **ARAÇ KULLANIMI GEREKTİĞİNDE:**
    - **TEK BİR ADIM PLANLA:** Önce, yapacağın *sadece bir sonraki* eylemi '<dusunce>...</dusunce>' etiketleri içinde açıkla.
    - **TEK BİR ARAÇ ÇAĞIR:** Ardından, *sadece bir tane* araç çağrısı için gerekli JSON objesini '<arac_cagrisi>...</arac_cagrisi>' etiketleri içine yerleştir.
    - **DUR VE BEKLE:** Araç çağrısını yaptıktan sonra cevabını bitir. Sistem sana aracın sonucunu "Araç-Sonucu: [sonuç]" formatında geri verecektir. Bir sonraki adıma geçmeden önce bu sonucu bekle. Kendi kendine araç sonucu üretme veya varsayma.
2.  **SOHBET GEREKTİĞİNDE (Araç Kullanımı Yoksa):**
    - Kullanıcının isteği bir araç kullanımı gerektirmiyorsa (örn: "merhaba", "nasılsın?"), SADECE ve SADECE düz metin olarak cevap ver.
    - Bu durumda ASLA XML etiketi veya JSON kullanma.`

	toolsPrompt := tools.GenerateToolsPrompt()
	fullSystemPrompt := fmt.Sprintf("%s\n\n%s", baseSystemPrompt, toolsPrompt)

	for {
		userInput, _ := pterm.DefaultInteractiveTextInput.Show("Siz")

		if strings.ToLower(userInput) == "exit" || strings.ToLower(userInput) == "çıkış" {
			pterm.Info.Println("Görüşmek üzere!")
			break
		}

		conversationHistory += "Kullanıcı: " + userInput + "\n"

		finalPrompt := fmt.Sprintf(`%s

--- Önceki Konuşma ---
%s
---------------------

Kullanıcı İsteği: %s`, fullSystemPrompt, conversationHistory, userInput)

		spinner, _ := pterm.DefaultSpinner.Start("Yapay zeka düşünüyor...")
		responseStr, err := ollama.Generate(cfg.Ollama.URL, cfg.Ollama.Model, finalPrompt)
		if err != nil {
			spinner.Fail(fmt.Sprintf("Ollama'dan cevap alınamadı: %v", err))
			continue
		}
		spinner.Success("Cevap alındı!")

		dusunce := extractTagContent(responseStr, "dusunce")
		aracCagrisiJSON := extractTagContent(responseStr, "arac_cagrisi")

		// AI'nın <arac_cagrisi> etiketini unuttuğu ancak yine de bir JSON döndürdüğü durumları yakala
		if aracCagrisiJSON == "" && strings.HasPrefix(strings.TrimSpace(responseStr), "{") {
			pterm.Warning.Println("AI, '<arac_cagrisi>' etiketini kullanmayı unuttu, ancak bir JSON nesnesi döndürdü. Yine de işlenmeye çalışılıyor.")
			aracCagrisiJSON = responseStr
		}

		if aracCagrisiJSON != "" {
			if dusunce != "" {
				pterm.DefaultBox.WithTitle("AI Düşünce Süreci").WithTextStyle(pterm.NewStyle(pterm.FgLightMagenta)).Println(dusunce)
			}

			var aiResponse ollama.AIResponse
			err = json.Unmarshal([]byte(aracCagrisiJSON), &aiResponse)
			if err != nil {
				pterm.Error.Printf("Araç çağrısı JSON formatında değil veya hatalı: %v\nGelen JSON: %s\n", err, aracCagrisiJSON)
				conversationHistory += "Asistan: [Hata: Geçersiz formatta araç çağrısı alındı.]\n"
				continue
			}

			if aiResponse.Type == "tool_call" {
				handleToolCall(aiResponse.ToolCall, &conversationHistory, aracCagrisiJSON, dusunce)
			} else {
				pterm.Error.Printf("Beklenmedik cevap tipi: '%s'\n", aiResponse.Type)
				conversationHistory += "Asistan: [Hata: Beklenmedik bir cevap tipi döndü.]\n"
			}
		} else {
			// AI'nin yanlışlıkla "Asistan:" ön ekini eklemesi durumunu temizle
			cleanResponse := strings.TrimSpace(responseStr)
			cleanResponse = strings.TrimPrefix(cleanResponse, "Asistan:")
			cleanResponse = strings.TrimSpace(cleanResponse)

			pterm.DefaultBasicText.WithStyle(pterm.NewStyle(pterm.FgLightCyan)).Println(cleanResponse)
			conversationHistory += "Asistan: " + cleanResponse + "\n"
		}
	}
}
