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

func main() {
	// 1. Yapılandırmayı Yükle
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		pterm.Fatal.Printf("Yapılandırma yüklenemedi: %v\n", err)
	}

	var conversationHistory string

	pterm.DefaultBigText.WithLetters(pterm.NewLettersFromString("GO AGENT")).Render()
	pterm.Info.Println("Go ile yazılmış, modüler ve yapılandırılabilir AI asistanı.")
	pterm.Println()

	// 2. Sistem Prompt'unu Araçlardan Dinamik Olarak Oluştur
	baseSystemPrompt := `Sen, Go dilinde yazılmış bir program tarafından kullanılan bir yapay zeka asistanısın.

# TEMEL KURALLARIN:
1.  **VARSAYILAN GÖREVİN SOHBET ETMEKTİR.** Kullanıcı sadece "merhaba", "nasılsın", "kimsin" gibi sohbet amaçlı şeyler söylediğinde veya belirsiz bir istekte bulunduğunda, ASLA araç kullanma. Normal bir şekilde sohbet ederek cevap ver.
2.  **ARAÇLARI SADECE AÇIK BİR KOMUT OLDUĞUNDA KULLAN.** Bir araç kullanman için, kullanıcının isteğinin araç açıklamalarından biriyle şüpheye yer bırakmayacak şekilde AÇIKÇA eşleşmesi gerekir. Örneğin: "dosyaları listele", "bu dosyayı oku", "saat kaç?"
3.  **ŞÜPHEDE KALDIĞINDA SOHBET ET.** Bir isteğin araç kullanımı gerektirip gerektirmediğinden emin değilsen, RİSK ALMA. Araç kullanmak yerine kullanıcıya soru sorarak veya sohbet ederek cevap ver.
4.  Araç kullanman gerektiğine karar verdiğinde, SADECE ve SADECE aşağıdaki gibi bir JSON bloğu ile cevap ver, başka hiçbir metin ekleme.
JSON formatı: {"tool_name": "kullanılacak_araç", "params": {"parametre_adı": "değer"}}`

	toolsPrompt := tools.GenerateToolsPrompt()
	fullSystemPrompt := fmt.Sprintf("%s\n\n%s", baseSystemPrompt, toolsPrompt)

	// 3. Ana Uygulama Döngüsü
	for {
		userInput, _ := pterm.DefaultInteractiveTextInput.Show("Siz")

		if strings.ToLower(userInput) == "exit" || strings.ToLower(userInput) == "çıkış" {
			pterm.Info.Println("Görüşmek üzere!")
			break
		}

		finalPrompt := fmt.Sprintf(`%s

--- Önceki Konuşma ---
%s
---------------------

Kullanıcı İsteği: %s`, fullSystemPrompt, conversationHistory, userInput)

		spinner, _ := pterm.DefaultSpinner.Start("Yapay zeka düşünüyor...")

		// 4. Ollama İsteğini Gönder
		responseStr, err := ollama.Generate(cfg.Ollama.URL, cfg.Ollama.Model, finalPrompt)
		if err != nil {
			spinner.Fail(fmt.Sprintf("Ollama'dan cevap alınamadı: %v", err))
			continue
		}
		spinner.Success("Cevap alındı!")

		conversationHistory += "Kullanıcı: " + userInput + "\n"
		responseStr = strings.TrimSpace(responseStr)

		// 5. Cevabı İşle: Araç mı, Sohbet mi?
		var toolCall ollama.ToolCall
		// LLM'in ürettiği JSON'u daha esnek ayrıştırmak için
		startIndex := strings.Index(responseStr, "{")
		lastIndex := strings.LastIndex(responseStr, "}")
		if startIndex != -1 && lastIndex != -1 && lastIndex > startIndex {
			jsonBlock := responseStr[startIndex : lastIndex+1]
			err = json.Unmarshal([]byte(jsonBlock), &toolCall)
		} else {
			err = fmt.Errorf("cevapta JSON bloğu bulunamadı")
		}

		if err == nil && toolCall.ToolName != "" {
			// Araç Kullanımı
			handleToolCall(toolCall, &conversationHistory)
		} else {
			// Normal Sohbet
			pterm.DefaultBasicText.WithStyle(pterm.NewStyle(pterm.FgLightCyan)).Println(responseStr)
			conversationHistory += "Asistan: " + responseStr + "\n"
		}
	}
}

func handleToolCall(toolCall ollama.ToolCall, conversationHistory *string) {
	pterm.Info.Println("LLM bir araç kullanmak istiyor:", toolCall.ToolName)
	pterm.Info.Println("Parametreler:", toolCall.Params)

	tool, exists := tools.ToolRegistry[toolCall.ToolName]
	if !exists {
		pterm.Error.Println("Bilinmeyen araç isteği:", toolCall.ToolName)
		*conversationHistory += "Asistan: [Hata: Bilinmeyen bir araç istendi.]\n"
		return
	}

	// Onay iste - GÜÇLENDİRİLMİŞ UYARI
	msg := fmt.Sprintf("'%s' aracını şu parametrelerle çalıştırmayı onaylıyor musunuz: %v", tool.Name, toolCall.Params)

	switch tool.Name {
	case "write_file":
		msg = pterm.Warning.Sprintf("DİKKAT: '%s' aracını çalıştırmak '%s' dosyasının üzerine yazabilir/değiştirebilir. Onaylıyor musunuz?", tool.Name, toolCall.Params["path"])
	case "run_shell_command":
		commandToRun := toolCall.Params["command"]
		// SUDO İÇİN EKSTRA UYARI
		if strings.Contains(commandToRun, "sudo") {
			msg = pterm.Error.Sprintf("!!! AŞIRI TEHLİKELİ İŞLEM !!! '%s' komutunu 'sudo' ile çalıştırmak üzeresiniz. Bu, sisteminizde kalıcı değişiklikler yapabilir veya zarar verebilir. Emin misiniz?", commandToRun)
		} else {
			msg = pterm.Warning.Sprintf("DİKKAT: '%s' komutunu terminalde çalıştırmak üzeresiniz. Onaylıyor musunuz?", commandToRun)
		}
	}

	approved, _ := pterm.DefaultInteractiveConfirm.Show(msg)

	if !approved {
		pterm.Warning.Println("İşlem iptal edildi.")
		*conversationHistory += "Asistan: [Kullanıcı tarafından araç kullanımı iptal edildi.]\n"
		return
	}

	// Aracı çalıştır
	result, err := tool.Execute(toolCall.Params)
	if err != nil {
		pterm.Error.Println("Araç hatası:", err)
		*conversationHistory += fmt.Sprintf("Asistan: [Araç çalıştırılırken hata oluştu: %v]\n", err)
	} else {
		pterm.DefaultBox.WithTitle("Araç Çıktısı: " + tool.Name).Println(result)
		*conversationHistory += fmt.Sprintf("Asistan: [Araç kullanıldı: %s, Sonuç: %s]\n", tool.Name, result)
	}
}