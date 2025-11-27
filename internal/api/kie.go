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
	
	var statusStr string
	
	if val, ok := unifiedResp.Data.Status.(string); ok {
		statusStr = val
	} else if val, ok := unifiedResp.Data.Status.(float64); ok {
		if val == 1 { statusStr = "SUCCESS" } else if val == 0 { statusStr = "GENERATING" } else { statusStr = "FAILED" }
	} 
	
	if statusStr == "" && unifiedResp.Data.SuccessFlag != nil {
		flag := *unifiedResp.Data.SuccessFlag
		if flag == 1 { 
			statusStr = "SUCCESS" 
		} else if flag == 0 { 
			statusStr = "GENERATING" 
		} else { 
			statusStr = "FAILED" 
		}
	}

	if statusStr == "" {
		statusStr = unifiedResp.Data.State
	}

	if statusStr == "SUCCESS" || statusStr == "success" {
		queryResp.Data.State = "success"
		
		var urls []string
		if len(unifiedResp.Data.Response.ResultUrls) > 0 {
			urls = unifiedResp.Data.Response.ResultUrls
		} else if len(unifiedResp.Data.Info.ResultUrls) > 0 {
			urls = unifiedResp.Data.Info.ResultUrls
		} else if unifiedResp.Data.ResultJSON != "" {
			queryResp.Data.ResultJSON = unifiedResp.Data.ResultJSON
			return &queryResp, nil
		}

		if len(urls) > 0 {
			resJSON, _ := json.Marshal(map[string][]string{"resultUrls": urls})
			queryResp.Data.ResultJSON = string(resJSON)
		}
	} else if statusStr == "GENERATING" || statusStr == "PENDING" || statusStr == "waiting" {
		queryResp.Data.State = "waiting"
	} else {
		queryResp.Data.State = "fail"
		errMsg := unifiedResp.Data.ErrorMessage
		if errMsg == "" { errMsg = unifiedResp.Data.FailMsg }
		if errMsg == "" { errMsg = "Unknown error / Flag Failed" }
		queryResp.Data.FailMsg = errMsg
	}

	return &queryResp, nil
}