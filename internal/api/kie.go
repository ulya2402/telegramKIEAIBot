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
		BaseURL: "https://api.kie.ai/api/v1",		
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second, // Naikkan timeout jadi 60 detik
		},
	}
}

func (c *KieClient) CreateTaskComplex(prompt string, modelName string, options map[string]interface{}) (string, error) {
	getOpt := func(key string, def string) string {
		if val, ok := options[key]; ok {
			if strVal, ok := val.(string); ok { return strVal }
		}
		return def
	}

	getArrayOpt := func(key string) []string {
		if val, ok := options[key]; ok {
			if list, ok := val.([]interface{}); ok {
				var res []string
				for _, item := range list {
					if str, ok := item.(string); ok { res = append(res, str) }
				}
				return res
			}
			if listString, ok := val.([]string); ok { return listString }
		}
		return []string{}
	}

	var targetURL string
	var jsonData []byte
	var err error

	// --- BRANCHING LOGIC ---

	if modelName == "gpt-4o-image" {
		targetURL = c.BaseURL + "/gpt4o-image/generate"
		reqBody := map[string]interface{}{
			"prompt": prompt,
			"size":   getOpt("ratio", "1:1"),
		}
		images := getArrayOpt("image_input")
		if len(images) > 0 {
			reqBody["filesUrl"] = images
		}
		jsonData, err = json.Marshal(reqBody)

	} else {
		targetURL = c.BaseURL + "/jobs/createTask"
		inputMap := make(map[string]interface{})
		inputMap["prompt"] = prompt

		if modelName == "google/nano-banana" {
			inputMap["output_format"] = getOpt("format", "png")
			inputMap["image_size"] = getOpt("ratio", "1:1") 
		} else if modelName == "nano-banana-pro" {
			inputMap["output_format"] = getOpt("format", "png")
			inputMap["aspect_ratio"] = getOpt("ratio", "1:1")
			inputMap["resolution"] = getOpt("resolution", "1K")
			inputMap["image_input"] = getArrayOpt("image_input")
		} else if modelName == "google/nano-banana-edit" {
			inputMap["output_format"] = getOpt("format", "png")
			inputMap["image_size"] = getOpt("ratio", "1:1")
			inputMap["image_urls"] = getArrayOpt("image_input")
		} else {
			inputMap["output_format"] = "png"
		}

		reqBody := models.KieTaskRequest{
			Model: modelName,
			Input: inputMap,
		}
		jsonData, err = json.Marshal(reqBody)
	}

	if err != nil { return "", err }

	req, err := http.NewRequest("POST", targetURL, bytes.NewBuffer(jsonData))
	if err != nil { return "", err }

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil { return "", err }
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		log.Printf("API Error Status: %d | Body: %s", resp.StatusCode, string(bodyBytes))
		return "", fmt.Errorf("API error status %d", resp.StatusCode)
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

// Ganti signature fungsi: Tambahkan parameter modelName
func (c *KieClient) GetTaskStatus(taskID string, modelName string) (*models.KieQueryResponse, error) {
	var url string

	if modelName == "gpt-4o-image" {
		// FIX: Endpoint yang benar sesuai dokumentasi
		url = fmt.Sprintf("%s/gpt4o-image/record-info?taskId=%s", c.BaseURL, taskID)
	} else {
		// Endpoint Banana Standard
		url = fmt.Sprintf("%s/jobs/recordInfo?taskId=%s", c.BaseURL, taskID)
	}
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil { return nil, err }
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("polling error %d", resp.StatusCode)
	}

	var queryResp models.KieQueryResponse
	
	// --- MAPPING MANUAL RESPONSE GPT-4o ---
	// Karena struktur JSON GPT-4o sedikit berbeda dengan Banana
	// (Banana: data.state | GPT: data.status)
	// Kita perlu 'menormalisasi' datanya agar struct models.KieQueryResponse tetap bisa dipakai.
	
	if modelName == "gpt-4o-image" {
		var gptResp struct {
			Code int `json:"code"`
			Data struct {
				Status   string `json:"status"` // SUCCESS
				Response struct {
					ResultUrls []string `json:"resultUrls"`
				} `json:"response"`
				ErrorMessage string `json:"errorMessage"`
			} `json:"data"`
		}
		
		if err := json.Unmarshal(bodyBytes, &gptResp); err != nil {
			return nil, err
		}

		// Normalisasi ke struct standard Banana kita
		queryResp.Code = gptResp.Code
		
		// Map Status
		if gptResp.Data.Status == "SUCCESS" {
			queryResp.Data.State = "success"
			// Bungkus resultUrls jadi JSON string agar sama logicnya dengan Banana
			resJSON, _ := json.Marshal(map[string][]string{"resultUrls": gptResp.Data.Response.ResultUrls})
			queryResp.Data.ResultJSON = string(resJSON)
		} else if gptResp.Data.Status == "GENERATING" {
			queryResp.Data.State = "waiting"
		} else if gptResp.Data.Status == "GENERATE_FAILED" || gptResp.Data.Status == "CREATE_TASK_FAILED" {
			queryResp.Data.State = "fail"
			queryResp.Data.FailMsg = gptResp.Data.ErrorMessage
		}

	} else {
		// Logic Banana Standard (Langsung unmarshal)
		json.Unmarshal(bodyBytes, &queryResp)
	}

	return &queryResp, nil
}