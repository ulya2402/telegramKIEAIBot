package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"kieAITelegram/internal/models"
	"log"
	"net/http"
	"time"
)

type KieClient struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

func NewKieClient(apiKey string) *KieClient {
	return &KieClient{
		APIKey:  apiKey,
		BaseURL: "https://api.kie.ai/api/v1/jobs",
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *KieClient) CreateTaskComplex(prompt string, modelName string, options map[string]interface{}) (string, error) {
	inputMap := make(map[string]interface{})
	inputMap["prompt"] = prompt

	// Helper untuk mengambil String Option
	getOpt := func(key string, def string) string {
		if val, ok := options[key]; ok {
			if strVal, ok := val.(string); ok {
				return strVal
			}
		}
		return def
	}

	// Helper untuk mengambil Array Option
	getArrayOpt := func(key string) []string {
		if val, ok := options[key]; ok {
			if list, ok := val.([]interface{}); ok {
				var res []string
				for _, item := range list {
					if str, ok := item.(string); ok {
						res = append(res, str)
					}
				}
				return res
			}
			if listString, ok := val.([]string); ok {
				return listString
			}
		}
		return []string{}
	}

	// --- LOGIC MAPPING PARAMETER (BAGIAN YANG BERUBAH) ---

	if modelName == "google/nano-banana" {
		// STANDARD
		inputMap["output_format"] = getOpt("format", "png")
		inputMap["image_size"] = getOpt("ratio", "1:1") 
	
	} else if modelName == "nano-banana-pro" {
		// PRO
		inputMap["output_format"] = getOpt("format", "png")
		inputMap["aspect_ratio"] = getOpt("ratio", "1:1") // Nama param: aspect_ratio
		inputMap["resolution"] = getOpt("resolution", "1K")
		inputMap["image_input"] = getArrayOpt("image_input") // Nama param: image_input

	} else if modelName == "google/nano-banana-edit" {
		// EDIT (Perhatikan perbedaan nama parameternya)
		inputMap["output_format"] = getOpt("format", "png")
		inputMap["image_size"] = getOpt("ratio", "1:1") // Nama param: image_size
		inputMap["image_urls"] = getArrayOpt("image_input") // Nama param: image_urls
	
	} else {
		// Default Fallback
		inputMap["output_format"] = "png"
	}

	// --- END LOGIC ---

	reqBody := models.KieTaskRequest{
		Model: modelName,
		Input: inputMap,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	// Debug Log untuk memastikan parameter benar
	log.Printf("[DEBUG API] Sending Payload to %s: %s", modelName, string(jsonData))

	req, err := http.NewRequest("POST", c.BaseURL+"/createTask", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		log.Printf("[ERROR API] Status: %d, Body: %s", resp.StatusCode, string(bodyBytes))
	}

	var kieResp models.KieTaskResponse
	if err := json.Unmarshal(bodyBytes, &kieResp); err != nil {
		return "", fmt.Errorf("API parse error: %s", string(bodyBytes))
	}

	if kieResp.Code != 200 {
		return "", fmt.Errorf("API error %d: %s", kieResp.Code, kieResp.Msg)
	}

	return kieResp.Data.TaskID, nil
}

func (c *KieClient) GetTaskStatus(taskID string) (*models.KieQueryResponse, error) {
	url := fmt.Sprintf("%s/recordInfo?taskId=%s", c.BaseURL, taskID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	var queryResp models.KieQueryResponse
	json.Unmarshal(bodyBytes, &queryResp)

	return &queryResp, nil
}