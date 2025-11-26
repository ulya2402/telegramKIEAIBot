package models

type Config struct {
	TelegramToken string
	KieAPIKey     string
	DBPath        string
	DefaultLang   string
}

type UserSession struct {
	UserID       int64
	LanguageCode string
}

type KieTaskRequest struct {
	Model string         `json:"model"`
	Input map[string]any `json:"input"`
}

type KieTaskResponse struct {
	Code int `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		TaskID string `json:"taskId"`
	} `json:"data"`
}

type KieQueryResponse struct {
	Code int `json:"code"`
	Data struct {
		State      string `json:"state"`
		ResultJSON string `json:"resultJson"`
		FailMsg    string `json:"failMsg"`
	} `json:"data"`
}

type KieResultJSON struct {
	ResultURLs []string `json:"resultUrls"`
}