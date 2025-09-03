package main

import (
	"fmt"
	"log"

	"go-agent/pkg/config"
	"go-agent/pkg/memory"
	"go-agent/pkg/ollama"

	"github.com/google/uuid"
)

func main() {
	// 1. Yapılandırmayı Yükle
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("FATAL: Yapılandırma yüklenemedi: %v", err)
	}
	fmt.Println("INFO: Yapılandırma başarıyla yüklendi.")

	// 2. ChromaDB İstemcisini Oluştur
	chromaClient := memory.NewChromaClient(cfg.Chroma.URL, cfg.Chroma.CollectionName)
	fmt.Println("INFO: ChromaDB istemcisi oluşturuldu.")

	// --- VERİ YAZMA (ADD) TESTİ ---
	fmt.Println("\n--- VERİ YAZMA TESTİ BAŞLATIYOR ---")
	testDocument := "Bu, doğrudan HTTP API ile yazılan bir test verisidir."
	testID := uuid.New().String() // Her seferinde benzersiz bir ID oluştur

	// 3. Test verisi için embedding oluştur
	fmt.Printf("INFO: '%s' için embedding oluşturuluyor...\n", testDocument)
	embedding, err := ollama.GenerateEmbedding(cfg.Ollama.URL, cfg.Ollama.EmbeddingModel, testDocument)
	if err != nil {
		log.Fatalf("FATAL: Embedding oluşturulamadı: %v", err)
	}
	fmt.Println("SUCCESS: Embedding başarıyla oluşturuldu.")

	// 4. Veriyi ChromaDB'ye ekle
	fmt.Printf("INFO: ID '%s' ile doküman veritabanına ekleniyor...\n", testID)
	err = chromaClient.Add(testID, embedding, testDocument)
	if err != nil {
		log.Fatalf("FATAL: Veritabanına eklenemedi: %v", err)
	}
	fmt.Println("SUCCESS: Doküman başarıyla veritabanına eklendi!")

	// --- VERİ OKUMA (QUERY) TESTİ ---
	fmt.Println("\n--- VERİ OKUMA TESTİ BAŞLATIYOR ---")

	// 5. Aynı embedding ile sorgulama yap
	fmt.Println("INFO: Az önce eklenen doküman geri sorgulanıyor...")
	retrievedDoc, err := chromaClient.Query(embedding, 1, cfg.Chroma.SimilarityThreshold)
	if err != nil {
		log.Fatalf("FATAL: Veritabanı sorgulanamadı: %v", err)
	}

	// 6. Sonucu Kontrol Et
	if retrievedDoc == "" {
		log.Fatalf("FAILURE: Sorgu bir sonuç döndürmedi! Veritabanı okuma başarısız.")
	}

	if retrievedDoc == testDocument {
		fmt.Println("\nSUCCESS: TEST BAŞARILI! Yazılan veri başarıyla geri okundu.")
		fmt.Printf("Gelen Veri: %s\n", retrievedDoc)
	} else {
		log.Fatalf("FAILURE: Okunan veri, yazılan veriyle eşleşmiyor! Gelen: '%s', Beklenen: '%s'", retrievedDoc, testDocument)
	}
}
