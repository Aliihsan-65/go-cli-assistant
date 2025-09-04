package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/pterm/pterm"
)

// Chroma'ya veri ekleme isteği için kullanılan yapı
type ChromaAddRequest struct {
	IDs        []string            `json:"ids"`
	Embeddings [][]float32         `json:"embeddings"`
	Metadatas  []map[string]string `json:"metadatas"`
	Documents  []string            `json:"documents" // Anlamsal aramanın yapılacağı metin`
}

// Chroma'dan veri sorgulama isteği için kullanılan yapı
type ChromaQueryRequest struct {
	QueryEmbeddings [][]float32 `json:"query_embeddings"`
	NResults        int         `json:"n_results"`
	Include         []string    `json:"include" // "metadatas" ve "distances" alanlarını getirmek için`
}

// Chroma sorgu cevabı için kullanılan yapı
type ChromaQueryResponse struct {
	IDs       [][]string            `json:"ids"`
	Distances [][]float64           `json:"distances"`
	Metadatas [][]map[string]string `json:"metadatas"`
}

// Chroma'dan tüm verileri çekme isteği için kullanılan yapı
type ChromaGetRequest struct {
	Include []string `json:"include"`
}

// Chroma'dan tüm verileri çekme cevabı için kullanılan yapı
type ChromaGetResponse struct {
	IDs       []string            `json:"ids"`
	Metadatas []map[string]string `json:"metadatas"`
	Documents []string            `json:"documents"`
}

// Koleksiyon oluşturma ve alma için kullanılan yapılar
type ChromaCreateCollectionRequest struct {
	Name string `json:"name"`
}

type ChromaGetCollectionResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ChromaClient, ChromaDB ile etkileşim için istemci
type ChromaClient struct {
	BaseURL        string
	CollectionName string
	CollectionID   string
	HTTPClient     *http.Client
}

// NewChromaClient, yeni bir ChromaClient oluşturur ve koleksiyonun varlığını garantiler.
func NewChromaClient(baseURL, collectionName string) *ChromaClient {
	client := &ChromaClient{
		BaseURL:        baseURL,
		CollectionName: collectionName,
		HTTPClient:     &http.Client{},
	}
	if err := client.ensureCollectionExists(); err != nil {
		pterm.Fatal.Printf("Hafıza koleksiyonu sağlanamadı: %v\n", err)
	}
	return client
}

// ensureCollectionExists, ChromaDB'de belirtilen koleksiyonun var olup olmadığını kontrol eder, yoksa oluşturur.
func (c *ChromaClient) ensureCollectionExists() error {
	createURL := c.BaseURL + "/api/v2/tenants/default_tenant/databases/default_database/collections"
	reqBody := ChromaCreateCollectionRequest{Name: c.CollectionName}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("koleksiyon oluşturma isteği JSON'a çevrilemedi: %w", err)
	}

	resp, err := c.HTTPClient.Post(createURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("koleksiyon oluşturma API hatası: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("koleksiyon cevap gövdesi okunamadı: %w", err)
	}
	responseBody := string(bodyBytes)

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		pterm.Success.Printf("Hafıza koleksiyonu '%s' başarıyla oluşturuldu veya doğrulandı.\n", c.CollectionName)
	case http.StatusConflict:
		pterm.Info.Printf("Hafıza koleksiyonu '%s' zaten mevcut.\n", c.CollectionName)
	default:
		if strings.Contains(responseBody, "already exists") {
			pterm.Info.Printf("Hafıza koleksiyonu '%s' zaten mevcut (sunucu hatasıyla tespit edildi).\n", c.CollectionName)
		} else {
			return fmt.Errorf("koleksiyon oluşturulamadı, durum: %s, cevap: %s", resp.Status, responseBody)
		}
	}

	getURL := c.BaseURL + "/api/v2/tenants/default_tenant/databases/default_database/collections/" + c.CollectionName
	getResp, err := c.HTTPClient.Get(getURL)
	if err != nil {
		return fmt.Errorf("koleksiyon bilgisi alınamadı: %w", err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		return fmt.Errorf("koleksiyon bilgisi alınamadı, durum: %s", getResp.Status)
	}

	bodyBytes, err = io.ReadAll(getResp.Body)
	if err != nil {
		return fmt.Errorf("koleksiyon cevap gövdesi okunamadı: %w", err)
	}

	var collectionInfo ChromaGetCollectionResponse
	if err := json.Unmarshal(bodyBytes, &collectionInfo); err != nil {
		return fmt.Errorf("koleksiyon bilgisi JSON'u çözümlenemedi: %w", err)
	}

	c.CollectionID = collectionInfo.ID
	pterm.Info.Printf("Hafıza koleksiyonu '%s' (ID: %s) hazır.\n", c.CollectionName, c.CollectionID)
	return nil
}

// Add, bir "dersi" (kullanıcı isteği ve doğru JSON komutu) veritabanına ekler.
func (c *ChromaClient) Add(id string, embedding []float32, userRequest string, toolCallJSON string) error {
	url := c.BaseURL + "/api/v2/tenants/default_tenant/databases/default_database/collections/" + c.CollectionID + "/add"

	reqBody := ChromaAddRequest{
		IDs:        []string{id},
		Embeddings: [][]float32{embedding},
		Documents:  []string{userRequest},
		Metadatas:  []map[string]string{{"user_request": userRequest, "tool_call_json": toolCallJSON}},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("add isteği JSON'a çevrilemedi: %w", err)
	}

	resp, err := c.HTTPClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("add API hatası: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("hafızaya eklenemedi, durum: %s, cevap: %s", resp.Status, string(body))
	}

	return nil
}

// QueryExamples, anlamsal olarak en yakın "dersleri" ve mesafelerini veritabanından çeker.
func (c *ChromaClient) QueryExamples(embedding []float32, topN int) ([]map[string]string, []float64, error) {
	url := c.BaseURL + "/api/v2/tenants/default_tenant/databases/default_database/collections/" + c.CollectionID + "/query"

	reqBody := ChromaQueryRequest{
		QueryEmbeddings: [][]float32{embedding},
		NResults:        topN,
		Include:         []string{"metadatas", "distances"},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("query isteği JSON'a çevrilemedi: %w", err)
	}

	resp, err := c.HTTPClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, nil, fmt.Errorf("query API hatası: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("query cevap gövdesi okunamadı: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("hafıza sorgulanamadı, durum: %s, cevap: %s", resp.Status, string(bodyBytes))
	}

	var queryResp ChromaQueryResponse
	if err := json.Unmarshal(bodyBytes, &queryResp); err != nil {
		return nil, nil, fmt.Errorf("hafıza cevabı çözümlenemedi: %w, Gelen Cevap: %s", err, string(bodyBytes))
	}

	if queryResp.Metadatas == nil || len(queryResp.Metadatas) == 0 || len(queryResp.Metadatas[0]) == 0 {
		return []map[string]string{}, []float64{}, nil
	}

	return queryResp.Metadatas[0], queryResp.Distances[0], nil
}

// GetAllExamples, veritabanındaki tüm kaydedilmiş "dersleri" çeker.
func (c *ChromaClient) GetAllExamples() ([]map[string]string, error) {
	url := c.BaseURL + "/api/v2/tenants/default_tenant/databases/default_database/collections/" + c.CollectionID + "/get"

	reqBody := ChromaGetRequest{Include: []string{"metadatas"}}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("get isteği JSON'a çevrilemedi: %w", err)
	}

	resp, err := c.HTTPClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("get API hatası: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("get cevap gövdesi okunamadı: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hafıza listelenemedi, durum: %s, cevap: %s", resp.Status, string(bodyBytes))
	}

	var getResp ChromaGetResponse
	if err := json.Unmarshal(bodyBytes, &getResp); err != nil {
		return nil, fmt.Errorf("hafıza listeleme cevabı çözümlenemedi: %w, Gelen Cevap: %s", err, string(bodyBytes))
	}

	if getResp.Metadatas == nil {
		return []map[string]string{}, nil
	}

	return getResp.Metadatas, nil
}
