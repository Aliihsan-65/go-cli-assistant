# go-agent Projesi Dokümantasyonu

Bu doküman, `go-agent` projesinin mimarisini, kullanılan teknolojileri, katmanlı yapısını ve temel çalışma prensiplerini açıklamaktadır.

## 1. Genel Bakış

`go-agent`, Go programlama dili ile geliştirilmiş, yerel (local) Büyük Dil Modelleri (LLM) ile çalışabilen, modüler ve genişletilebilir bir yapay zeka asistanıdır. Temel amacı, kullanıcıdan gelen doğal dil komutlarını anlayarak, önceden tanımlanmış araçları (tools) kullanmak ve siber güvenlik, pentest otomasyonu gibi görevleri yerine getirmektir. Ayrıca genel sohbet yeteneğine de sahiptir.

Proje, uzun süreli hafıza yeteneği için **RAG (Retrieval-Augmented Generation)** mimarisini kullanır. Bu sayede, geçmişte öğrenilen bilgilerden faydalanarak daha doğru ve bağlama uygun çıktılar üretebilir.

## 2. Kullanılan Teknolojiler

- **Programlama Dili:** Go
- **LLM Sağlayıcısı:** [Ollama](https://ollama.com/) (Yerel modelleri çalıştırmak için)
    - **Akıl Yürütme (Reasoning) Modeli:** `Cybersecurity-BaronLLM`
    - **Embedding Modeli:** `nomic-embed-text`
- **Vektör Veritabanı (RAG için):** [ChromaDB](https://www.trychroma.com/)
- **Terminal Arayüzü:** [pterm](https://github.com/pterm/pterm) (Renkli ve interaktif CLI için)
- **Yapılandırma:** YAML (`config.yaml`)
- **Bağımlılık Yönetimi:** Go Modules (`go.mod`)

## 3. Proje Mimarisi ve Katmanlar

Proje, standart bir Go proje yapısını takip ederek `cmd` (uygulama giriş noktası) ve `pkg` (paylaşılan paketler/kütüphaneler) olarak iki ana bölüme ayrılmıştır. Bu yapı, kodun modüler, anlaşılır ve sürdürülebilir olmasını sağlar.

```
/
├── cmd/
│   └── go-agent/
│       └── main.go       # Uygulamanın giriş noktası, ana döngü
├── pkg/
│   ├── config/
│   │   └── config.go     # YAML yapılandırmasını yükler
│   ├── memory/
│   │   └── chroma.go     # RAG mimarisi, ChromaDB istemcisi
│   ├── ollama/
│   │   └── client.go     # Ollama API istemcisi (LLM ve embedding)
│   └── tools/
│       └── tools.go      # Agent'ın kullanabileceği araçların tanımı
├── config.yaml           # Proje yapılandırması
├── go.mod                # Proje bağımlılıkları
└── README.md             # Bu doküman
```

### Katmanların Açıklaması

1.  **`cmd/go-agent/main.go` (Uygulama Katmanı):**
    -   Uygulamanın ana giriş noktasıdır.
    -   `config` paketini kullanarak `config.yaml` dosyasını okur.
    -   `ollama` ve `memory` (Chroma) istemcilerini başlatır.
    -   Kullanıcıdan komutları okuyan, ana uygulama döngüsünü (REPL) yöneten katmandır.
    -   LLM'den gelen cevabı analiz eder, araç çağrısı olup olmadığını belirler ve `tools` paketini kullanarak ilgili aracı çalıştırır.
    -   Kullanıcı etkileşimlerini (`pterm` ile) yönetir.

2.  **`pkg/config` (Yapılandırma Katmanı):**
    -   `config.yaml` dosyasını okuyup bir Go `struct`'ına (`Config`) dönüştürmekten sorumludur.
    -   Ollama model adı, API adresleri, ChromaDB koleksiyon adı gibi merkezi ayarları sağlar.

3.  **`pkg/ollama` (LLM İstemci Katmanı):**
    -   Ollama API'si ile iletişimi soyutlar.
    -   `Generate`: Verilen bir prompt ile LLM'den metin tabanlı bir cevap üretir.
    -   `GenerateEmbedding`: Verilen bir metni, anlamsal arama için kullanılacak bir vektöre (embedding) dönüştürür.

4.  **`pkg/tools` (Araç Katmanı):**
    -   Agent'ın yeteneklerini tanımlar. `ToolRegistry` adında bir `map` içerisinde tüm araçları barındırır.
    -   Her `Tool`; bir isim, açıklama (LLM'e rehberlik etmek için) ve `Execute` fonksiyonu içerir.
    -   `run_shell_command`, `read_file`, `write_file` gibi temel araçlar burada tanımlanmıştır.
    -   `GenerateToolsPrompt`: LLM'e, kullanabileceği araçların listesini ve açıklamalarını dinamik olarak sunan bir prompt metni oluşturur.

5.  **`pkg/memory` (Hafıza/RAG Katmanı):**
    -   Projenin uzun süreli hafızasını yönetir. Bu katman, RAG mimarisinin temelini oluşturur.
    -   Detayları bir sonraki bölümde açıklanmıştır.

## 4. RAG (Retrieval-Augmented Generation) Mimarisi

Proje, LLM'in sadece anlık bağlamla sınırlı kalmamasını, geçmiş deneyimlerden öğrenmesini sağlamak için RAG mimarisini kullanır. Bu, `/eğit` komutu ile tetiklenen "dersler" aracılığıyla gerçekleştirilir.

### Çalışma Prensibi

1.  **Öğrenme (Training - `/eğit`):**
    -   Kullanıcı, agent'a yeni bir şey öğretmek istediğinde `/eğit` komutunu kullanır.
    -   Agent'a bir görev verilir (örn: "mevcut dizini listele").
    -   Agent bir çıktı üretir (`TOOL_NAME: run_shell_command, TOOL_PARAMS: ls -l`).
    -   Kullanıcı bu çıktının doğruluğunu onaylar ve tekrar `/eğit` komutunu çalıştırarak bu "dersi" kaydeder.
    -   Bu noktada `chroma.go` içindeki `Add` fonksiyonu devreye girer:
        a.  Kullanıcının orijinal isteği ("mevcut dizini listele"), `ollama.GenerateEmbedding` kullanılarak bir vektöre dönüştürülür.
        b.  Bu vektör, orijinal istek metni ve doğru araç çıktısı (`TOOL_NAME...`) ile birlikte ChromaDB'ye kaydedilir.

2.  **Akıl Yürütme (Inference):**
    -   Kullanıcı yeni bir komut girdiğinde (örn: "dosyaları göster"), `main.go` döngüsü çalışır.
    -   `chroma.go` içindeki `QueryExamples` fonksiyonu devreye girer:
        a.  Yeni komut ("dosyaları göster"), yine `ollama.GenerateEmbedding` ile bir vektöre çevrilir.
        b.  Bu yeni vektör, ChromaDB'de anlamsal olarak en benzer kayıtları (geçmiş dersleri) bulmak için bir sorgu olarak kullanılır.
        c.  ChromaDB, en benzer "dersleri" (örn: "mevcut dizini listele" ve onun başarılı çıktısı) döndürür.
    -   Bu geri alınan örnekler, LLM'e gönderilen ana prompt'a "BAŞARILI ÖRNEKLER" başlığı altında eklenir.
    -   LLM, bu örneklerden öğrenerek, yeni gelen "dosyaları göster" isteğinin de muhtemelen `run_shell_command` ile `ls -l` komutunu çalıştırmak anlamına geldiğini çıkarır ve doğru çıktıyı üretir.

Bu sayede agent, zamanla daha akıllı hale gelir ve benzer görevleri daha önce öğrendiği başarılı yöntemleri kullanarak çözer. Bu mimari, agent'ın yetenek setini statik araç tanımlarının ötesine taşır ve dinamik bir öğrenme süreci sağlar.
