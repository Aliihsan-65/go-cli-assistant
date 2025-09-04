# Go-Agent Gelişmiş Hafıza Mimarisi: "Akıl Hocası" Modeli

## 1. Tespit Edilen Sorun ve Amaç

Basit bir vektör veritabanı önbelleği (cache), anlamsal olarak birbirine çok benzeyen ancak kritik parametreleri (IP adresi, port, dosya adı vb.) farklı olan isteklerde tehlikeli bir şekilde hatalı davranabilir. Örneğin, "8.8.8.8 port 80'i tara" komutunu hafızaya aldıktan sonra, "8.8.8.8 port 443'ü tara" isteği geldiğinde, anlamsal benzerlikten ötürü hafızadaki eski komutu döndürerek yanlış portun taranmasına neden olabilir.

Asıl amaç, basit bir önbellek mekanizması kurmak değil, yapay zekayı zamanla **eğitmek ve düzeltmektir**. Kullanıcı, yapay zekanın bir hatasını düzelttiğinde veya başarılı bir sonucunu onayladığında, bu "dersin" sisteme kaydedilerek gelecekteki benzer görevlerde daha doğru sonuçlar üretmesi hedeflenmektedir.

## 2. Çözüm: "Akıl Hocası" (Few-Shot Learning) Yaklaşımı

Bu hedefi gerçekleştirmek için vektör veritabanı (ChromaDB), bir cevap deposu olarak değil, bir **başarılı örnekler deposu** olarak kullanılacaktır. Yapay zeka (LLM) karar mekanizmasından tamamen çıkarılmaz, aksine geçmişteki başarılı örneklerle beslenerek daha doğru kararlar vermesi sağlanır.

### Çalışma Mimarisi

1.  **Hafızaya Kaydetme (Öğretme):**
    Kullanıcı, bir araç çağrısının sonucundan memnun kaldığında, bu işlemi hafızaya kaydeder. Her kayıt, iki temel bilgiyi içerir:
    *   **Kullanıcı İsteği:** Komutun verilmesine neden olan orijinal, doğal dil metni (örn: "verbose modda detaylı bir nmap taraması yap").
    *   **Doğru JSON Çıktısı:** Bu istek için üretilmiş olan doğru ve tam JSON araç çağrısı (örn: `{"type":"tool_call","tool_call":{"tool_name":"run_shell_command","params":{"command":"nmap -A -vvv ..."}}}`).

2.  **Hafızadan Faydalanma (Öğrenme ve Uygulama):**
    *   Kullanıcı "Araç Modu"nda yeni bir istekte bulunduğunda, sistem bu isteği vektöre çevirerek ChromaDB'de anlamsal olarak en yakın bir veya birkaç kayıtlı örneği bulur.
    *   Bulunan bu başarılı örnekler, o anki görev için LLM'e gönderilecek olan sistem talimatına (prompt) dinamik olarak eklenir.

3.  **Yönlendirilmiş Prompt Yapısı:**
    LLM'e gönderilen son talimat şu yapıda olur:

    ```
    SEN, bir siber güvenlik uzmanısın... (Ana kurallar)

    # BAŞARILI ÖRNEKLER
    # Geçmişte doğru olarak çözülmüş bu örneklerden öğrenerek yeni görevi tamamla.
    # Örnek 1:
    #   Kullanıcı İsteği: "verbose tarama yap"
    #   Üretilen Doğru Komut: {"type":"tool_call", ... "command":"nmap -v ..."}

    # YENİ GÖREV
    # Yukarıdaki başarılı örneklerden yola çıkarak, aşağıdaki yeni kullanıcı isteği için doğru JSON komutunu üret:
    # Yeni Kullanıcı İsteği: "192.168.1.1 için çok verbose bir tarama yap"
    ```

### Sonuç ve Faydalar

*   **Parametre Ezme Sorunu Çözülür:** LLM, eski örneğe bakarak `verbose` için `-v` kullanması gerektiğini öğrenir, ancak **yeni isteği** de okuyarak hedef IP'yi doğru şekilde günceller. Körü körüne eski komutu kopyalamaz.
*   **Sürekli Öğrenme:** Sistem, her başarılı ve kaydedilmiş işlemle daha akıllı hale gelir.
*   **Güvenilirlik:** Hafıza, körü körüne cevap veren bir mekanizma yerine, LLM'e yol gösteren bir **akıl hocası** görevi görür.

## 3. Teknik Notlar

### ChromaDB API Versiyon Notu

Proje, modern bir ChromaDB versiyonu ile çalışacak şekilde ayarlanmıştır. Bu versiyon, daha spesifik ve `tenant`/`database` içeren `/api/v2/` API yollarını kullanmayı gerektirir.

- **Kullanılması Gereken API Yolu Yapısı:** `/api/v2/tenants/{tenant}/databases/{database}/collections`
- **Varsayılan Değerler:** Kod içinde `tenant` için `default_tenant` ve `database` için `default_database` kullanılmaktadır.

**Önemli Not:** Program başlangıcında `405 Method Not Allowed` hatası alınırsa, bu durum büyük ihtimalle `pkg/memory/chroma.go` dosyasındaki API yollarının yanlış olduğunu gösterir. Yolların yukarıda belirtilen `v2` yapısını kullandığından emin olunmalıdır. `/api/v1/collections` gibi daha eski veya genel yollar bu hataya neden olmaktadır.