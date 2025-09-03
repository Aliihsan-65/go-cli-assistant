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

type ChromaAddRequest struct {
	IDs        []string            `json:"ids"`
	Embeddings [][]float32         `json:"embeddings"`
	Metadatas  []map[string]string `json:"metadatas"`
	Documents  []string            `json:"documents"`
}

type ChromaQueryRequest struct {
	QueryEmbeddings [][]float32 `json:"query_embeddings"`
	NResults        int         `json:"n_results"`
}

type ChromaQueryResponse struct {
	IDs       [][]string            `json:"ids"`
	Distances [][]float64           `json:"distances"`
	Metadatas [][]map[string]string `json:"metadatas"`
	Documents [][]string            `json:"documents"`
}

type ChromaCreateCollectionRequest struct {
	Name string `json:"name"`
}

type ChromaGetCollectionResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ChromaClient struct {
	BaseURL        string
	CollectionName string
	CollectionID   string
	HTTPClient     *http.Client
}

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
	case http.StatusInternalServerError:
		if strings.Contains(responseBody, "already exists") {
			pterm.Info.Printf("Hafıza koleksiyonu '%s' zaten mevcut (500 hatasıyla tespit edildi).\n", c.CollectionName)
		} else {
			return fmt.Errorf("koleksiyon oluşturulamadı, durum: %s, cevap: %s", resp.Status, responseBody)
		}
	default:
		return fmt.Errorf("beklenmedik durum, kod: %d, cevap: %s", resp.StatusCode, responseBody)
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

func (c *ChromaClient) Add(id string, embedding []float32, document string) error {
	url := c.BaseURL + "/api/v2/tenants/default_tenant/databases/default_database/collections/" + c.CollectionID + "/add"

	reqBody := ChromaAddRequest{
		IDs:        []string{id},
		Embeddings: [][]float32{embedding},
		Metadatas:  []map[string]string{{"document": document}},
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

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("add cevap gövdesi okunamadı: %w", err)
	}
	responseBody := string(bodyBytes)

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("hafızaya eklenemedi, durum: %s, cevap: %s", resp.Status, responseBody)
	}

	return nil
}

func (c *ChromaClient) Query(embedding []float32, topN int, threshold float64) (string, error) {
	url := c.BaseURL + "/api/v2/tenants/default_tenant/databases/default_database/collections/" + c.CollectionID + "/query"

	reqBody := ChromaQueryRequest{
		QueryEmbeddings: [][]float32{embedding},
		NResults:        topN,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("query isteği JSON'a çevrilemedi: %w", err)
	}

	resp, err := c.HTTPClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("query API hatası: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("query cevap gövdesi okunamadı: %w", err)
	}
	responseBody := string(bodyBytes)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("hafıza sorgulanamadı, durum: %s, cevap: %s", resp.Status, responseBody)
	}

	var queryResp ChromaQueryResponse
	if err := json.Unmarshal(bodyBytes, &queryResp); err != nil {
		return "", fmt.Errorf("hafıza cevabı çözümlenemedi: %w, Gelen Cevap: %s", err, responseBody)
	}

	if len(queryResp.IDs) > 0 && len(queryResp.IDs[0]) > 0 && len(queryResp.Distances) > 0 && len(queryResp.Distances[0]) > 0 && queryResp.Distances[0][0] < (1-threshold) {
		return queryResp.Metadatas[0][0]["document"], nil
	}

	return "", nil
}
