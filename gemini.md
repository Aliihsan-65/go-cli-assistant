# Go Agent Projesi: Gemini ile Geliştirme Serüveni

Bu dosya, Gemini ile birlikte sıfırdan geliştirdiğimiz `go-agent` adlı yapay zeka asistanının tüm geliştirme sürecini, karşılaşılan zorlukları ve öğrenilenleri belgelemektedir.

## 1. Projenin Amacı

Projenin temel amacı, kullanıcının yerel makinesinde çalışan, terminal üzerinden komut alabilen, dosya sistemi üzerinde (listeleme, okuma, yazma) işlemler yapabilen ve bu işlemleri yapmadan önce kullanıcıdan onay alan, Go dilinde yazılmış bir yapay zeka asistanı oluşturmaktı.

## 2. Kullanılan Teknolojiler

- **Programlama Dili:** Go
- **LLM Altyapısı:** Ollama
- **Kullanılan Modeller:** `gemma3`, `deepseek-r1:8b`
- **Terminal Arayüzü (TUI):** `pterm` kütüphanesi

## 3. Çalışma Mantığı

Asistan, sonsuz bir döngü içinde kullanıcıdan komut alır ve aşağıdaki mantıkla çalışır:

1.  **System Prompt (Sistem Talimatı):** Her kullanıcı girdisi, LLM'e gönderilmeden önce, ona rolünü, yeteneklerini, kurallarını ve elindeki araçları tanımlayan büyük bir talimat metniyle birleştirilir.
2.  **Araçlar (Tools):** `list_directory`, `read_file`, `write_file` gibi yetenekler, Go içinde normal fonksiyonlar olarak tanımlanmıştır.
3.  **JSON ile İletişim:** LLM, bir aracı kullanmaya karar verdiğinde, sohbet etmek yerine, hangi aracı hangi parametrelerle kullanmak istediğini belirten bir JSON metni döndürür.
4.  **Akıllı Ayrıştırma:** Go programı, LLM'den gelen cevabın içinde bir JSON bloğu arar. Eğer bulursa, bunu bir araç çağrısı olarak işler. Bulamazsa, normal bir sohbet mesajı olarak kabul eder.
5.  **Hafıza (Conversation History):** Program, konuşma geçmişini (kullanıcı girdileri ve asistan cevapları) bir değişkende tutar. Bir sonraki komutta bu geçmişi de LLM'e göndererek, asistanın önceki konuşmalardaki bağlamı hatırlaması sağlanır. Araç kullanıldığında, hafızaya ham JSON yerine, daha anlaşılır bir özet kaydedilir.
6.  **Görsel Arayüz:** `pterm` kütüphanesi kullanılarak, bekleme animasyonları (spinner), renkli ve ikonlu bilgilendirme/hata mesajları, kutu içinde çıktılar ve interaktif onay mekanizmaları ile kullanıcı deneyimi zenginleştirilmiştir.

## 4. Geliştirme Sürecinde Öğrenilenler

- **Prompt Engineering'in Önemi:** LLM'in davranışını yönlendirmenin en etkili yolunun `systemPrompt`'u doğru ve net bir şekilde yazmak olduğu görüldü. Denge çok önemliydi:
    - Çok katı talimatlar, LLM'in sohbet yeteneğini kaybetmesine neden oldu.
    - Çok esnek talimatlar, LLM'in araçları kullanmak yerine halüsinasyon görmesine (görevi yaptığını iddia etmesine) neden oldu.
    - Parametre çıkarımı için talimatlara net örnekler (`Örneğin: ...`) eklemenin modelin performansını ciddi şekilde artırdığı gözlemlendi.
- **Hafıza Yönetimi:** Stateless (hafızasız) bir asistanın kullanışlı olmadığı anlaşıldı. Konuşma geçmişi eklendi. Daha sonra, bu geçmişe ham JSON yerine, işlenmiş özetlerin eklenmesinin, LLM'in bağlamı daha iyi anlamasına yardımcı olduğu keşfedildi.
- **Go Dilinin İncelikleri:** Geliştirme sırasında `switch-case` bloklarındaki değişken kapsamı (scope) kuralları ve `string` (metin) oluşturma kuralları gibi Go diline özgü syntax yapıları üzerinde pratik yapıldı ve hatalar düzeltildi.
- **Kütüphane Kullanımı:** `pterm` gibi harici bir kütüphaneyi kullanırken, dokümantasyonuna veya doğru kullanım şekline hakim olmanın ne kadar önemli olduğu, deneme-yanılma yoluyla tecrübe edildi.

## 5. Kurulum ve Çalıştırma

- **Geliştirme Ortamında Çalıştırma:**
  ```bash
  cd /home/un1c4on/go-agent
  go run main.go
  ```
- **Sistem Geneli Komut Olarak Kurma:**
  ```bash
  cd /home/un1c4on/go-agent
  go build -o go-agent
  sudo cp go-agent /usr/local/bin/
  ```

## 6. Sorun Giderme ve Yapılandırma Notları

Bu bölüm, proje canlıya alındıktan sonra karşılaşılan sorunları ve kalıcı çözümlerini içerir.

### ChromaDB Kurulumu ve Yönetimi

Projenin hafıza mekanizması olarak kullandığı ChromaDB, bir Docker container'ı olarak çalışmaktadır.

#### İlk Kurulum

ChromaDB container'ını ilk defa oluşturmak ve başlatmak için aşağıdaki komut kullanılır. Bu komut, `go-agent-chroma` adında bir container oluşturur, 8000 portunu haritalar ve verilerin kalıcı olması için `chroma_data` adında bir volume kullanır:

```bash
docker run -d --name go-agent-chroma -p 8000:8000 -v chroma_data:/chroma/.chroma/index chromadb/chroma
```

#### Yeniden Başlatma

Bilgisayar yeniden başlatıldıktan sonra durmuş olan ChromaDB container'ını tekrar başlatmak için `docker-compose` veya `docker run` komutlarına gerek yoktur. Sadece aşağıdaki komut yeterlidir:

```bash
docker start go-agent-chroma
```

### API ve Kod Değişiklikleri

#### ChromaDB API v2 Entegrasyonu

Geliştirme sırasında, kullanılan `chromadb/chroma` Docker imajının, Go kodunun başlangıçta kullandığı `v1` API'sini terk ettiği ve `v2` API'sine geçtiği tespit edildi. Sunucudan gelen `The v1 API is deprecated. Please use /v2 apis` hatası bu durumu ortaya çıkardı.

Çözüm olarak, `pkg/memory/chroma.go` dosyasındaki tüm API çağrı yolları, aşağıdaki yeni yapıya uygun şekilde güncellendi:

`/api/v2/tenants/default_tenant/databases/default_database/`

Bu, `default_tenant` ve `default_database` varsayılan adları kullanılarak yapılmıştır.

### Yapay Zeka Davranışsal Ayarlama (Prompt Engineering)

Modelin istenmeyen davranışlarını (sohbet için araç kullanma, genel kültür soruları için `web_search` aracına başvurma) düzeltmek için bir dizi değişiklik yapıldı.

1.  **Sistem Komutu (System Prompt) Güncellemeleri:** `cmd/go-agent/main.go` dosyasındaki `baseSystemPrompt` değişkeni, modelin ne zaman araç kullanıp ne zaman kendi bilgisine başvurması gerektiğini netleştiren katı kurallarla birkaç kez güncellendi.
2.  **Araç Tanımlarının İyileştirilmesi:** `pkg/tools/tools.go` dosyasındaki `web_search` aracının tanımı ve örnekleri, modeli yanlış yönlendirdiği için güncellendi.
3.  **`web_search` Aracının Kaldırılması:** Modelin `web_search` kullanma alışkanlığının çok güçlü olduğu görüldü. Bu nedenle, modeli kendi iç bilgeliğini kullanmaya zorlamak amacıyla, kullanıcı isteği üzerine `web_search` aracı `pkg/tools/tools.go` dosyasındaki `ToolRegistry`'den tamamen kaldırıldı.