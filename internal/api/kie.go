package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"kieAITelegram/internal/models"
	"log"
	"net/http"
	"strings"
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
			Timeout: 60 * time.Second,
		},
	}
}

func (c *KieClient) CreateTaskComplex(prompt string, modelName string, options map[string]interface{}) (string, error) {
	getOpt := func(key string, def string) string {
		if val, ok := options[key]; ok {
			if strVal, ok := val.(string); ok {
				return strVal
			}
		}
		return def
	}

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

	var targetURL string
	var jsonData []byte
	var err error

	if modelName == "veo3" || modelName == "veo3_fast" {
		targetURL = c.BaseURL + "/veo/generate"
		
		reqBody := map[string]interface{}{
			"model":       modelName,
			"prompt":      prompt,
			"aspectRatio": getOpt("ratio", "16:9"),
		}
		
		images := getArrayOpt("image_input")
		if len(images) > 0 {
			reqBody["imageUrls"] = images
			if len(images) == 1 {
				reqBody["generationType"] = "FIRST_AND_LAST_FRAMES_2_VIDEO" 
			} else if len(images) >= 2 {
				reqBody["generationType"] = "FIRST_AND_LAST_FRAMES_2_VIDEO"
			}
		} else {
			reqBody["generationType"] = "TEXT_2_VIDEO"
		}
		
		jsonData, err = json.Marshal(reqBody)

	} else if modelName == "gpt-4o-image" { 
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
		} else if modelName == "qwen/image-edit" {
			// --- QWEN IMAGE EDIT (LOGIKA BARU) ---
			targetURL = c.BaseURL + "/jobs/createTask"
			
			inputMap := make(map[string]interface{})
			inputMap["prompt"] = prompt
			
			// 1. Ambil gambar input. Qwen butuh "image_url" (string), bukan array.
			images := getArrayOpt("image_input")
			if len(images) > 0 {
				inputMap["image_url"] = images[len(images)-1] // Ambil gambar terakhir
			} else {
				return "", fmt.Errorf("model ini wajib menyertakan upload gambar")
			}
	
			// 2. Set parameter khusus sesuai dokumentasi Qwen
			inputMap["image_size"] = getOpt("ratio", "landscape_4_3") // Mapping ratio -> image_size
			inputMap["output_format"] = getOpt("format", "png")
			inputMap["acceleration"] = "none"
			inputMap["num_inference_steps"] = 25
			inputMap["guidance_scale"] = 4
			inputMap["enable_safety_checker"] = true
			inputMap["negative_prompt"] = "blurry, ugly"
	
			reqBody := models.KieTaskRequest{
				Model: modelName,
				Input: inputMap,
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

	if err != nil {
		return "", err
	}


	req, err := http.NewRequest("POST", targetURL, bytes.NewBuffer(jsonData))
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

func (c *KieClient) GetTaskStatus(taskID string, modelName string) (*models.KieQueryResponse, error) {
	var url string
	if strings.Contains(modelName, "veo") {
		url = fmt.Sprintf("%s/veo/record-info?taskId=%s", c.BaseURL, taskID)
	} else if strings.Contains(modelName, "gpt-4o") {
		url = fmt.Sprintf("%s/gpt4o-image/record-info?taskId=%s", c.BaseURL, taskID)
	} else {
		url = fmt.Sprintf("%s/jobs/recordInfo?taskId=%s", c.BaseURL, taskID)
	}

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

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("polling error %d", resp.StatusCode)
	}

	var queryResp models.KieQueryResponse

	// Struktur respons unified untuk menangkap berbagai format field
	var unifiedResp struct {
		Code int `json:"code"`
		Data struct {
			Status      interface{} `json:"status"`
			SuccessFlag *int        `json:"successFlag"`
			State       string      `json:"state"`

			Response struct {
				ResultUrls []string `json:"resultUrls"`
			} `json:"response"`
			Info struct {
				ResultUrls []string `json:"resultUrls"`
			} `json:"info"`

			ResultJSON   string `json:"resultJson"`
			ErrorMessage string `json:"errorMessage"`
			FailMsg      string `json:"failMsg"`
		} `json:"data"`
	}

	if err := json.Unmarshal(bodyBytes, &unifiedResp); err != nil {
		return nil, err
	}

	queryResp.Code = unifiedResp.Code

	// --- LOGIKA PENENTUAN STATUS YANG LEBIH PINTAR ---
	finalState := "fail" // Default awal

	// Helper untuk cek string case-insensitive
	isSuccess := func(s string) bool {
		s = strings.ToLower(s)
		return s == "success" || s == "finished" || s == "done" || s == "complete"
	}
	isWaiting := func(s string) bool {
		s = strings.ToLower(s)
		return s == "waiting" || s == "pending" || s == "generating" || s == "queue" || s == "processing"
	}

	// 1. Cek State String (Biasanya paling akurat di dokumentasi modern)
	if isSuccess(unifiedResp.Data.State) {
		finalState = "success"
	} else if isWaiting(unifiedResp.Data.State) {
		finalState = "waiting"
	}

	// 2. Cek Status Field (Interface: bisa string atau number)
	// Hanya cek jika belum confirm success/waiting
	if finalState == "fail" {
		if val, ok := unifiedResp.Data.Status.(string); ok {
			if isSuccess(val) {
				finalState = "success"
			} else if isWaiting(val) {
				finalState = "waiting"
			}
		} else if val, ok := unifiedResp.Data.Status.(float64); ok {
			if val == 1 {
				finalState = "success"
			} else if val == 0 {
				finalState = "waiting"
			}
		}
	}

	// 3. Cek SuccessFlag (Legacy field)
	if finalState == "fail" && unifiedResp.Data.SuccessFlag != nil {
		flag := *unifiedResp.Data.SuccessFlag
		if flag == 1 {
			finalState = "success"
		} else if flag == 0 {
			finalState = "waiting"
		}
	}

	// --- PROSES HASIL BERDASARKAN STATUS ---
	queryResp.Data.State = finalState

	if finalState == "success" {
		var urls []string

		// Cek berbagai tempat kemungkinan URL hasil
		if len(unifiedResp.Data.Response.ResultUrls) > 0 {
			urls = unifiedResp.Data.Response.ResultUrls
		} else if len(unifiedResp.Data.Info.ResultUrls) > 0 {
			urls = unifiedResp.Data.Info.ResultUrls
		} else if unifiedResp.Data.ResultJSON != "" {
			// [FIX UTAMA UNTUK QWEN]
			// Jika ResultJSON ada isinya, langsung pakai itu.
			queryResp.Data.ResultJSON = unifiedResp.Data.ResultJSON
			return &queryResp, nil
		}

		if len(urls) > 0 {
			resJSON, _ := json.Marshal(map[string][]string{"resultUrls": urls})
			queryResp.Data.ResultJSON = string(resJSON)
		} else if queryResp.Data.ResultJSON == "" {
			// Kasus aneh: Sukses tapi tidak ada URL
			queryResp.Data.ResultJSON = "{}"
		}

	} else if finalState == "waiting" {
		// Do nothing, just return state waiting
	} else {
		// FAILED
		errMsg := unifiedResp.Data.ErrorMessage
		if errMsg == "" {
			errMsg = unifiedResp.Data.FailMsg
		}
		if errMsg == "" {
			// Tambahkan info debug state asli agar kita tahu kenapa gagal
			debugInfo := unifiedResp.Data.State
			if debugInfo == "" {
				debugInfo = "empty"
			}
			errMsg = fmt.Sprintf("Unknown error / Flag Failed (RawState: %s)", debugInfo)
		}
		queryResp.Data.FailMsg = errMsg
	}

	return &queryResp, nil
}