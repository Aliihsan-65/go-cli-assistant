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
	"os/exec"
	"strings"

	"github.com/google/uuid"
	"github.com/pterm/pterm"
)

// DDGResult, ddgr'ın JSON çıktısındaki tek bir arama sonucunu temsil eder.
type DDGResult struct {
	Title    string `json:"title"`
	Abstract string `json:"abstract"`
	URL      string `json:"url"`
}

// parseToolCall, LLM'in metin çıktısını analiz eder ve TOOL_NAME ile TOOL_PARAMS'ı çıkarır.
func parseToolCall(response string) (toolName string, toolParams string, err error) {
	scanner := bufio.NewScanner(strings.NewReader(response))
	var paramsBuilder strings.Builder
	inParamsSection := false

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "TOOL_NAME:") {
			toolName = strings.TrimSpace(strings.TrimPrefix(line, "TOOL_NAME:"))
			inParamsSection = false // Her ihtimale karşı
		} else if strings.HasPrefix(line, "TOOL_PARAMS:") {
			// TOOL_PARAMS'dan sonraki ilk satır
			inParamsSection = true
			paramsBuilder.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "TOOL_PARAMS:")))
		} else if inParamsSection {
			// TOOL_PARAMS'ın devam eden satırları
			paramsBuilder.WriteString("\n" + line)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", "", fmt.Errorf("yanıt okunurken hata: %w", err)
	}

	if toolName == "" {
		return "", "", fmt.Errorf("yanıtta 'TOOL_NAME:' bulunamadı")
	}

	toolParams = strings.TrimSpace(paramsBuilder.String())
	return toolName, toolParams, nil
}

// handleToolCall, bir araç çağrısını yönetir, çalıştırır, geçmişi kaydeder ve başarı durumunu döndürür.
func handleToolCall(toolName, toolParams string, conversationHistory *string, rawResponse string) bool {
	if toolName == "" {
		pterm.Error.Println("AI, bir araç çağırmaya çalıştı ancak hangisi olduğunu belirtmedi.")
		*conversationHistory += `Asistan: [Hata: İsimsiz bir araç çağrı isteği alındı.]\n`
		return false
	}

	// 'run_command' gibi eski veya yanlış isimleri düzelt
	if toolName == "run_command" {
		pterm.Warning.Println("AI, 'run_command' çağırmaya çalıştı, 'run_shell_command' olarak düzeltiliyor.")
		toolName = "run_shell_command"
	}

	pterm.Info.Println("LLM şu aracı kullanmak istiyor:", toolName)
	pterm.Info.Println("Ham Parametreler:", toolParams)

	t, exists := tools.ToolRegistry[toolName]
	if !exists {
		pterm.Error.Println("Bilinmeyen araç istendi:", toolName)
		*conversationHistory += `Asistan: [Hata: Bilinmeyen bir araç istendi.]\n`
		return false
	}

	// Parametreleri, onay mesajında göstermek ve aracı çalıştırmak için erkenden ayrıştır.
	parsedParams, err := tools.ParseParams(toolName, toolParams)
	if err != nil {
		pterm.Error.Println("Araç parametreleri ayrıştırılamadı:", err)
		*conversationHistory += fmt.Sprintf(`Asistan: [Hata: Parametre ayrıştırma hatası: %v]\n`, err)
		return false
	}

	msg := fmt.Sprintf("'%s' aracını şu parametrelerle çalıştırmayı onaylıyor musunuz: %v", t.Name, parsedParams)

	switch t.Name {
	case "write_file":
		msg = pterm.Warning.Sprintf(`DİKKAT: '%s' aracını çalıştırmak '%s' dosyasının üzerine yazabilir/değiştirebilir. Onaylıyor musunuz?`, t.Name, parsedParams["file_path"])
	case "append_file":
		msg = pterm.Warning.Sprintf(`DİKKAT: '%s' aracını çalıştırmak '%s' dosyasına ekleme yapacak. Onaylıyor musunuz?`, t.Name, parsedParams["file_path"])
	case "run_shell_command":
		commandToRun, exists := parsedParams["command"]
		if !exists {
			pterm.Error.Println("run_shell_command için 'command' parametresi eksik.")
			*conversationHistory += `Asistan: [Hata: 'command' parametresi eksik.]\n`
			return false
		}
		if strings.Contains(commandToRun, "sudo") {
			msg = pterm.Error.Sprintf(`!!! AŞIRI TEHLİKELİ İŞLEM !!! '%s' komutunu 'sudo' ile çalıştırmak üzeresiniz. Bu, sisteminizde kalıcı değişiklikler yapabilir veya zarar verebilir. Emin misiniz?`, commandToRun)
		} else {
			msg = pterm.Warning.Sprintf(`DİKKAT: '%s' komutunu terminalde çalıştırmak üzeresiniz. Onaylıyor musunuz?`, commandToRun)
		}
	}

	approved, _ := pterm.DefaultInteractiveConfirm.Show(msg)

	turnHistory := rawResponse + "\n"

	if !approved {
		pterm.Warning.Println("İşlem iptal edildi.")
		turnHistory += `Araç-Sonucu: [Kullanıcı tarafından iptal edildi.]\n`
		*conversationHistory += turnHistory
		return false
	}

	result, err := t.Execute(parsedParams)
	if err != nil {
		pterm.Error.Println("Araç hatası:", err)
		turnHistory += fmt.Sprintf(`Araç-Sonucu: [Hata: %v]\n`, err)
		*conversationHistory += turnHistory
		return false
	}

	pterm.DefaultBox.WithTitle("Araç Çıktısı: " + t.Name).Println(result)
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
		// Yeni formatta, "Üretilen Doğru Komut" birden fazla satır olabilir.
		// Bu yüzden her satırın başına "#   " ekleyerek formatı koruyoruz.
		formattedCmd := strings.ReplaceAll(ex["tool_call_json"], "\n", "\n#   ")
		builder.WriteString(fmt.Sprintf("#   Üretilen Doğru Komut:\n#   %s\n", formattedCmd))
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
	pterm.Info.Println("Sohbet için doğrudan yazın. Araç kullanmak için '/tool <isteğiniz>' yazın.")
	pterm.Println()

	reader := bufio.NewReader(os.Stdin)

	// Araç modu için gerekli değişkenleri döngü dışında tanımla
	toolPrompt := `SEN, bir siber güvenlik uzmanı ve komut satırı arayüzü (CLI) asistanısın. Görevin, kullanıcı isteklerini, SADECE sana verilen araçları kullanarak çözmektir.

# TEMEL KURALLAR
1.  Cevabın HER ZAMAN ve SADECE şu formatta olmalı:
    TOOL_NAME: <araç_adı>
    TOOL_PARAMS: <parametreler>
2.  ASLA açıklama, selamlama veya başka bir metin ekleme. Sadece TOOL_NAME ve TOOL_PARAMS ver.
3.  TOOL_NAME olarak SADECE "KULLANABİLECEĞİN ARAÇLAR" listesindekileri kullanabilirsin. ASLA bu listenin dışında bir araç adı (örneğin 'nmap', 'ls', 'cat') kullanma.
4.  Eğer kullanıcı nmap, ls, cat, echo gibi bir terminal komutu çalıştırmak istiyorsa, TOOL_NAME olarak HER ZAMAN run_shell_command kullanmalısın. TOOL_PARAMS ise komutun tamamı olmalıdır.

# ÖRNEKLER

## ÖRNEK 1: Terminal Komutu Çalıştırma
Kullanıcı İsteği: 216.150.1.193 adresine karşı agresif bir nmap taraması yap
SEN:
TOOL_NAME: run_shell_command
TOOL_PARAMS: nmap -A 216.150.1.193

## ÖRNEK 2: Dosya Yazma
Kullanıcı İsteği: bulgular.txt dosyasına 'Port 80 açık' yaz
SEN:
TOOL_NAME: write_file
TOOL_PARAMS: bulgular.txt "Port 80 açık"

## ÖRNEK 3: Dizin Listeleme
Kullanıcı İsteği: mevcut dizindeki dosyaları göster
SEN:
TOOL_NAME: run_shell_command
TOOL_PARAMS: ls -l

ŞİMDİ BAŞLA.`
	toolsListPrompt := tools.GenerateToolsPrompt()
	baseSystemPrompt := fmt.Sprintf("%s\n\n%s", toolPrompt, toolsListPrompt)
	var inTrainingMode = false
	var lastRequestForTraining = ""
	var lastSuccessfulOutputForTraining = ""

	// Sohbet modu için gerekli değişken
	chatPrompt := `SEN, yardımsever bir sohbet asistanısın. Görevin sadece kullanıcıyla sohbet etmektir. Asla ve asla özel formatlar, etiketler veya araçlar kullanma.`

	for {
		pterm.DefaultBasicText.Print(pterm.LightYellow("Siz: "))
		userInput, _ := reader.ReadString('\n')
		userInput = strings.TrimSpace(userInput)

		if strings.ToLower(userInput) == "exit" || strings.ToLower(userInput) == "çıkış" {
			break
		}

		// Mod seçimi
		if strings.HasPrefix(userInput, "/tool") {
			// ARAÇ KULLANIM MODU
			toolInput := strings.TrimSpace(strings.TrimPrefix(userInput, "/tool"))

			// Araç moduna özgü komutlar burada da çalışmalı
			if toolInput == "/web" { // /tool /web -> /web
				pterm.DefaultBasicText.Print(pterm.LightBlue("Web'de ne aramak istersiniz?: "))
				query, _ := reader.ReadString('\n')
				query = strings.TrimSpace(query)
				if query == "" {
					pterm.Warning.Println("Arama sorgusu boş olamaz.")
					continue
				}
				// ... (web arama mantığı aynen kalıyor)
				spinner, _ := pterm.DefaultSpinner.Start("Web'de aranıyor: ", query)
				cmd := exec.Command("ddgr", "--json", "-n", "5", query)
				output, err := cmd.CombinedOutput()
				spinner.Stop()
				if err != nil {
					pterm.Error.Printf("ddgr komutu çalıştırılamadı: %v\nÇıktı: %s\n", err, string(output))
					continue
				}
				var results []DDGResult
				if err := json.Unmarshal(output, &results); err != nil {
					pterm.Error.Printf("Arama sonuçları (JSON) ayrıştırılamadı: %v\n", err)
					continue
				}
				var historyBuilder strings.Builder
				historyBuilder.WriteString(fmt.Sprintf("Kullanıcı bir web araması yaptı. Sorgu: \"%s\". Bulunan sonuçlar:\n", query))
				pterm.Success.Printf("'%s' için %d sonuç bulundu:\n", query, len(results))
				for i, res := range results {
					pterm.DefaultBox.WithTitle(fmt.Sprintf("Sonuç %d: %s", i+1, res.Title)).Println(pterm.LightYellow("URL: ") + res.URL + "\n" + pterm.LightCyan("Özet: ") + res.Abstract)
					historyBuilder.WriteString(fmt.Sprintf("- Başlık: %s, URL: %s, Özet: %s\n", res.Title, res.URL, res.Abstract))
				}
				conversationHistory += historyBuilder.String()
				pterm.Info.Println("Arama sonuçları konuşma geçmişine eklendi.")
				continue
			}
			if toolInput == "/hafizayisifirla" {
				approved, _ := pterm.DefaultInteractiveConfirm.Show("DİKKAT: Bu işlem geri alınamaz. Tüm eğitim verilerini (hafızayı) kalıcı olarak silmek istediğinizden emin misiniz?")
				if approved {
					if err := chromaClient.DeleteCollection(); err != nil {
						pterm.Error.Printf("Hafıza silinirken bir hata oluştu: %v\n", err)
						continue
					}
					if err := chromaClient.CreateCollection(); err != nil {
						pterm.Error.Printf("Hafıza yeniden oluşturulurken bir hata oluştu: %v\n", err)
						continue
					}
					pterm.Success.Println("Hafıza başarıyla sıfırlandı.")
				} else {
					pterm.Warning.Println("Hafıza sıfırlama işlemi iptal edildi.")
				}
				continue
			}
			if toolInput == "/eğit" {
				if !inTrainingMode {
					inTrainingMode = true
					pterm.Info.Println("Eğitim modu başlatıldı. Lütfen öğretmek istediğiniz komutu girin. (Örn: /tool <komut>)")
					lastRequestForTraining = ""
					lastSuccessfulOutputForTraining = ""
				} else {
					if lastRequestForTraining != "" && lastSuccessfulOutputForTraining != "" {
						embedding, err := ollama.GenerateEmbedding(cfg.Ollama.URL, cfg.Ollama.EmbeddingModel, lastRequestForTraining)
						if err != nil {
							pterm.Warning.Printf("Hafızaya kaydetmek için embedding oluşturulamadı: %v\n", err)
						} else {
							uuid := uuid.New().String()
							err = chromaClient.Add(uuid, embedding, lastRequestForTraining, lastSuccessfulOutputForTraining)
							if err != nil {
								pterm.Warning.Printf("Ders hafızaya eklenemedi: %v\n", err)
							} else {
								pterm.Success.Println("Yeni ders hafızaya başarıyla eklendi!")
							}
						}
					} else {
						pterm.Warning.Println("Kaydedilecek başarılı bir komut bulunamadı.")
					}
					inTrainingMode = false
					lastRequestForTraining = ""
					lastSuccessfulOutputForTraining = ""
				}
				continue
			}
			if toolInput == "/showmemory" {
				//... (showmemory mantığı aynen kalıyor)
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
					content := fmt.Sprintf(`Kullanıcı İsteği: %s\nDoğru Komut:\n%s`, ex["user_request"], ex["tool_call_json"])
					pterm.DefaultBox.WithTitle(boxTitle).Println(content)
				}
				continue
			}
			if toolInput == "" {
				pterm.Warning.Println("Araç modu için bir istek girmelisiniz. Örnek: /tool mevcut dizini listele")
				continue
			}

			// Normal veya Eğitim modunda komut işleme
			inputEmbedding, err := ollama.GenerateEmbedding(cfg.Ollama.URL, cfg.Ollama.EmbeddingModel, toolInput)
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

			cwd, _ := os.Getwd()
			conversationHistory += "Kullanıcı: " + toolInput + "\n"
			finalPrompt := fmt.Sprintf(`%s
%s

# MEVCUT ÇALIŞMA DİZİNİ
%s

--- Önceki Konuşma ---
%s
---------------------

Kullanıcı İsteği: %s`, baseSystemPrompt, examplesText, cwd, conversationHistory, toolInput)

			spinner, _ := pterm.DefaultSpinner.Start("Uzman AI düşünüyor...")
			responseStr, err := ollama.Generate(cfg.Ollama.URL, cfg.Ollama.Model, finalPrompt)
			spinner.Stop()
			if err != nil {
				pterm.Error.Printf("Ollama'dan cevap alınamadı: %v", err)
				continue
			}

			toolName, toolParams, err := parseToolCall(responseStr)
			if err != nil {
				pterm.Warning.Println("AI'nın cevabı geçerli bir araç çağrısı formatında değil:", err)
				pterm.Println(responseStr)
				conversationHistory += "Asistan: [Hata: Araç çağrısı ayrıştırılamadı] " + responseStr + "\n"
				if inTrainingMode {
					pterm.Warning.Println("Eğitim modunda komut başarısız oldu. Moddan çıkılıyor.")
					inTrainingMode = false
				}
				continue
			}

			isSuccess := handleToolCall(toolName, toolParams, &conversationHistory, responseStr)
			if inTrainingMode {
				if isSuccess {
					lastRequestForTraining = toolInput
					lastSuccessfulOutputForTraining = responseStr
					pterm.Success.Println("Eğitim komutu başarıyla çalıştı. Kaydetmek için tekrar /eğit komutunu girin.")
				} else {
					pterm.Warning.Println("Eğitim sırasında komut başarısız oldu. Moddan çıkılıyor.")
					inTrainingMode = false
				}
			}

		} else {
			// GENEL SOHBET MODU
			cwd, _ := os.Getwd()
			conversationHistory += "Kullanıcı: " + userInput + "\n"
			finalPrompt := fmt.Sprintf(`%s

# MEVCUT ÇALIŞMA DİZİNİ
%s

--- Önceki Konuşma ---
%s
---------------------

Kullanıcı İsteği: %s`, chatPrompt, cwd, conversationHistory, userInput)

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
