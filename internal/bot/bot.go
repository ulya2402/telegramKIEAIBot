package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"kieAITelegram/internal/api"
	"kieAITelegram/internal/core"
	"kieAITelegram/internal/database"
	"kieAITelegram/internal/i18n"
	"kieAITelegram/internal/models"
	"log"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

type Bot struct {
	Token     string
	APIURL    string
	DB        *database.SQLiteDB
	KieClient *api.KieClient
	Localizer *i18n.Localizer
	Offset    int64
}

func NewBot(token string, db *database.SQLiteDB, kie *api.KieClient, loc *i18n.Localizer) *Bot {
	return &Bot{
		Token:     token,
		APIURL:    "https://api.telegram.org/bot" + token,
		DB:        db,
		KieClient: kie,
		Localizer: loc,
		Offset:    0,
	}
}

func (b *Bot) Start() {
	log.Println("Bot started polling...")
	for {
		updates, err := b.getUpdates()
		if err != nil {
			log.Printf("Error updates: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		for _, update := range updates {
			if update.UpdateID >= b.Offset {
				b.Offset = update.UpdateID + 1
			}
			go b.handleUpdate(update)
		}
		time.Sleep(1 * time.Second)
	}
}

func (b *Bot) getUpdates() ([]models.TelegramUpdate, error) {
	url := fmt.Sprintf("%s/getUpdates?offset=%d&timeout=60", b.APIURL, b.Offset)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result models.TelegramResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Result, nil
}

func (b *Bot) handleUpdate(u models.TelegramUpdate) {
	if u.CallbackQuery != nil {
		b.handleCallback(u.CallbackQuery)
		return
	}
	if u.Message != nil {
		if u.Message.Text != "" {
			b.handleMessage(u.Message)
		}
		if len(u.Message.Photo) > 0 {
			b.handlePhotoUpload(u.Message)
		}
	}
}

func (b *Bot) handleMessage(msg *models.TelegramMessage) {
	text := strings.TrimSpace(msg.Text)
	chatID := msg.Chat.ID
	userID := msg.From.ID

	if text == "/start" {
		b.DB.SetUserState(userID, "IDLE", "")
		b.sendMessage(chatID, "<b>Welcome!</b> Use /img to generate images.")
		return
	}

	if text == "/img" {
		b.showProviders(chatID, 0, false)
		return
	}

	state := b.DB.GetUserState(userID)
	
	if state.State == "WAITING_IMAGE_UPLOAD" {
		b.sendMessage(chatID, "🖼️ Please upload an image or click <b>Done</b>.")
		return
	}

	if state.State == "WAITING_PROMPT" && state.SelectedModel != "" {
		b.processImageGeneration(chatID, userID, text, state)
	} else {
		b.sendMessage(chatID, "Please use /img to start.")
	}
}

func (b *Bot) handlePhotoUpload(msg *models.TelegramMessage) {
	chatID := msg.Chat.ID
	userID := msg.From.ID
	state := b.DB.GetUserState(userID)

	if state.State != "WAITING_IMAGE_UPLOAD" {
		return 
	}

	bestPhoto := msg.Photo[len(msg.Photo)-1]
	fileURL, err := b.getFileDirectURL(bestPhoto.FileID)
	if err != nil {
		log.Printf("[ERROR] Failed to get file URL: %v", err)
		b.sendMessage(chatID, "❌ Failed to get image URL.")
		return
	}

	currentOpts := state.DraftOptions
	var imageList []string

	if existing, ok := currentOpts["image_input"]; ok {
		if listInterface, ok := existing.([]interface{}); ok {
			for _, item := range listInterface {
				if str, ok := item.(string); ok {
					imageList = append(imageList, str)
				}
			}
		} else if listString, ok := existing.([]string); ok {
			imageList = listString
		}
	}

	if len(imageList) >= 8 {
		b.sendMessage(chatID, "⚠️ Max 8 images allowed.")
		return
	}

	imageList = append(imageList, fileURL)
	b.DB.UpdateDraftOption(userID, "image_input", imageList)
	b.sendMessage(chatID, fmt.Sprintf("✅ <b>Image Received!</b> (%d/8)\nClick <b>Done</b> when finished.", len(imageList)))
}

func (b *Bot) handleCallback(cb *models.CallbackQuery) {
	parts := strings.SplitN(cb.Data, ":", 3)
	action := parts[0]
	chatID := cb.Message.Chat.ID
	messageID := cb.Message.MessageID
	userID := cb.From.ID

	http.Get(fmt.Sprintf("%s/answerCallbackQuery?callback_query_id=%s", b.APIURL, cb.ID))

	switch action {
	case "prov":
		if len(parts) > 1 {
			b.showModels(chatID, messageID, parts[1])
		}
	
	case "model":
		if len(parts) > 1 {
			modelID := parts[1]
			b.DB.SetUserState(userID, "WAITING_PROMPT", modelID)
			
			// RESET DEFAULTS
			b.DB.UpdateDraftOption(userID, "ratio", "1:1")
			b.DB.UpdateDraftOption(userID, "format", "png")
			b.DB.UpdateDraftOption(userID, "image_input", []string{}) 
			
			model := core.GetModelByID(modelID)
			for _, op := range model.SupportedOps {
				if op == "resolution" {
					b.DB.UpdateDraftOption(userID, "resolution", "1K")
				}
			}
			b.showModelDashboard(chatID, messageID, userID, modelID)
		}

	case "dash":
		if len(parts) > 1 {
			modelID := parts[1]
			b.DB.SetUserState(userID, "WAITING_PROMPT", modelID)
			b.showModelDashboard(chatID, messageID, userID, modelID)
		}

	case "set":
		if len(parts) > 1 {
			settingType := parts[1]
			if settingType == "image_input" {
				currentState := b.DB.GetUserState(userID)
				b.DB.SetUserState(userID, "WAITING_IMAGE_UPLOAD", currentState.SelectedModel) 
				
				kb := models.InlineKeyboardMarkup{
					InlineKeyboard: [][]models.InlineKeyboardButton{
						{{Text: "✅ Done / Selesai", CallbackData: "upload_done"}},
					},
				}
				b.editMessageWithKeyboard(chatID, messageID, "📤 <b>Upload Mode</b>\n\nSend photos now (Max 8).\nPress <b>Done</b> when finished.", kb)
			} else {
				b.showSettingOptions(chatID, messageID, userID, settingType)
			}
		}

	case "upload_done":
		state := b.DB.GetUserState(userID)
		b.DB.SetUserState(userID, "WAITING_PROMPT", state.SelectedModel)
		b.showModelDashboard(chatID, messageID, userID, state.SelectedModel)

	case "opt":
		if len(parts) > 2 {
			settingType := parts[1]
			value := parts[2]
			b.DB.UpdateDraftOption(userID, settingType, value)
			state := b.DB.GetUserState(userID)
			b.showModelDashboard(chatID, messageID, userID, state.SelectedModel)
		}

	case "back_home":
		b.showProviders(chatID, messageID, true)
	case "back_model":
		b.showModels(chatID, messageID, "google")
	}
}

func (b *Bot) getFileDirectURL(fileID string) (string, error) {
	url := fmt.Sprintf("%s/getFile?file_id=%s", b.APIURL, fileID)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Ok     bool `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", b.Token, result.Result.FilePath)
	return fileURL, nil
}

func (b *Bot) showProviders(chatID int64, messageID int64, isEdit bool) {
	var rows [][]models.InlineKeyboardButton
	for _, p := range core.AI_REGISTRY {
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: "📂 " + p.Name, CallbackData: "prov:" + p.ID},
		})
	}
	kb := models.InlineKeyboardMarkup{InlineKeyboard: rows}
	text := "<b>Select AI Provider:</b>"

	if isEdit {
		b.editMessageWithKeyboard(chatID, messageID, text, kb)
	} else {
		b.sendMessageWithKeyboard(chatID, text, kb)
	}
}

func (b *Bot) showModels(chatID int64, messageID int64, providerID string) {
	prov := core.GetProviderByID(providerID)
	if prov == nil {
		return
	}
	var rows [][]models.InlineKeyboardButton
	for _, m := range prov.Models {
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: "🤖 " + m.Name, CallbackData: "model:" + m.ID},
		})
	}
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "🔙 Back", CallbackData: "back_home"},
	})
	kb := models.InlineKeyboardMarkup{InlineKeyboard: rows}
	
	text := fmt.Sprintf("<b>Provider:</b> %s\nSelect Model:", prov.Name)
	b.editMessageWithKeyboard(chatID, messageID, text, kb)
}

func (b *Bot) showModelDashboard(chatID int64, messageID int64, userID int64, modelID string) {
	model := core.GetModelByID(modelID)
	state := b.DB.GetUserState(userID)
	opts := state.DraftOptions

	text := fmt.Sprintf("🚀 <b>Model:</b> %s\n", model.Name)
	text += fmt.Sprintf("📝 <b>Status:</b> <i>Waiting for Prompt...</i>\n\n")
	text += "⚙️ <b>Current Settings:</b>\n"
	text += "<pre>"
	
	for _, op := range model.SupportedOps {
		val, exists := opts[op]
		if !exists {
			val = "-"
		}
		
		if op == "image_input" {
			count := 0
			if list, ok := val.([]interface{}); ok {
				count = len(list)
			} else if list, ok := val.([]string); ok {
				count = len(list)
			}
			text += fmt.Sprintf("• Image Input: %d files\n", count)
		} else {
			text += fmt.Sprintf("• %-10s : %v\n", strings.Title(op), val)
		}
	}
	
	text += "</pre>\n"
	text += "👇 <i>Configure options below or just type your prompt:</i>"

	var rows [][]models.InlineKeyboardButton
	var row []models.InlineKeyboardButton

	for _, op := range model.SupportedOps {
		btnText := fmt.Sprintf("Set %s", strings.Title(op))
		if op == "image_input" {
			btnText = "🖼️ Upload Images"
		}
		
		row = append(row, models.InlineKeyboardButton{Text: btnText, CallbackData: "set:" + op})
		if len(row) == 2 {
			rows = append(rows, row)
			row = []models.InlineKeyboardButton{}
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}

	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "🔙 Back to Models", CallbackData: "back_model"},
	})

	kb := models.InlineKeyboardMarkup{InlineKeyboard: rows}
	b.editMessageWithKeyboard(chatID, messageID, text, kb)
}

func (b *Bot) showSettingOptions(chatID int64, messageID int64, userID int64, settingType string) {
	state := b.DB.GetUserState(userID)
	model := core.GetModelByID(state.SelectedModel)

	var options []string
	switch settingType {
	case "ratio":
		options = model.Ratios
	case "format":
		options = model.Formats
	case "resolution":
		options = model.Resolutions
	}

	var rows [][]models.InlineKeyboardButton
	var row []models.InlineKeyboardButton
	for _, opt := range options {
		row = append(row, models.InlineKeyboardButton{
			Text: opt,
			CallbackData: fmt.Sprintf("opt:%s:%s", settingType, opt),
		})
		if len(row) == 3 {
			rows = append(rows, row)
			row = []models.InlineKeyboardButton{}
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: "🔙 Back", CallbackData: "dash:" + model.ID},
	})

	kb := models.InlineKeyboardMarkup{InlineKeyboard: rows}
	text := fmt.Sprintf("<b>Select %s:</b>", strings.Title(settingType))
	b.editMessageWithKeyboard(chatID, messageID, text, kb)
}

func (b *Bot) processImageGeneration(chatID int64, userID int64, prompt string, state database.UserState) {
	model := core.GetModelByID(state.SelectedModel)
	if model == nil {
		b.sendMessage(chatID, "Error: Model not found.")
		return
	}
	b.sendMessage(chatID, fmt.Sprintf("🎨 Generating with <b>%s</b>...\nPrompt: <i>%s</i>", model.Name, prompt))
	go func() {
		log.Printf("[DEBUG] Starting Task with Options: %+v", state.DraftOptions)

		taskID, err := b.KieClient.CreateTaskComplex(prompt, model.APIModelID, state.DraftOptions)
		if err != nil {
			log.Printf("API Error: %v", err)
			b.sendMessage(chatID, "❌ Failed to start generation.")
			return
		}
		b.pollTaskResult(chatID, taskID)
	}()
}

func (b *Bot) pollTaskResult(chatID int64, taskID string) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	timeout := time.After(3 * time.Minute)
	for {
		select {
		case <-timeout:
			b.sendMessage(chatID, "⚠️ Timeout.")
			return
		case <-ticker.C:
			status, err := b.KieClient.GetTaskStatus(taskID)
			if err != nil {
				continue
			}
			
			if status.Data.State == "success" {
				log.Printf("[DEBUG] RESULT SUCCESS. Raw JSON: %s", status.Data.ResultJSON)

				var res models.KieResultJSON
				json.Unmarshal([]byte(status.Data.ResultJSON), &res)

				if len(res.ResultURLs) > 0 {
					imgURL := res.ResultURLs[0]
					// KITA GUNAKAN FUNGSI BARU UNTUK MENGIRIM FILE
					b.sendPhoto(chatID, imgURL, "Generated by KieAI")
				} else {
					log.Printf("[ERROR] ResultURLs empty!")
					b.sendMessage(chatID, "⚠️ Result URL is empty.")
				}
				return
			} else if status.Data.State == "fail" {
				b.sendMessage(chatID, "❌ Failed: "+status.Data.FailMsg)
				return
			}
		}
	}
}

// --- FUNGSI DOWNLOAD & UPLOAD (PROXY) ---
// Ini solusi untuk error "failed to get HTTP URL content"

func (b *Bot) sendPhoto(chatID int64, photoURL, caption string) {
	log.Printf("[PROXY] Downloading image: %s", photoURL)
	
	// 1. Download Gambar dari Kie AI
	resp, err := http.Get(photoURL)
	if err != nil {
		log.Printf("[PROXY ERROR] Download failed: %v", err)
		b.sendMessage(chatID, "❌ Error downloading generated image.")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("[PROXY ERROR] Download Status: %d", resp.StatusCode)
		b.sendMessage(chatID, "❌ Image server returned error.")
		return
	}

	// 2. Siapkan Multipart Form untuk Upload ke Telegram
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Field chat_id
	writer.WriteField("chat_id", fmt.Sprintf("%d", chatID))
	// Field caption
	writer.WriteField("caption", caption)
	
	// Field photo (File)
	// Kita namakan file-nya "image.png" agar telegram mengenali formatnya
	part, err := writer.CreateFormFile("photo", "image.png")
	if err != nil {
		log.Printf("[PROXY ERROR] CreateFormFile failed: %v", err)
		return
	}
	
	// Copy data download ke form upload
	_, err = io.Copy(part, resp.Body)
	if err != nil {
		log.Printf("[PROXY ERROR] Copy failed: %v", err)
		return
	}
	writer.Close()

	// 3. Upload ke Telegram
	uploadReq, err := http.NewRequest("POST", fmt.Sprintf("%s/sendPhoto", b.APIURL), body)
	if err != nil {
		log.Printf("[PROXY ERROR] NewRequest failed: %v", err)
		return
	}
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 60 * time.Second}
	uploadResp, err := client.Do(uploadReq)
	if err != nil {
		log.Printf("[PROXY ERROR] Upload to Telegram failed: %v", err)
		b.sendMessage(chatID, "❌ Failed to send image to Telegram.")
		return
	}
	defer uploadResp.Body.Close()

	// Cek Response Telegram
	respBody, _ := io.ReadAll(uploadResp.Body)
	if uploadResp.StatusCode != 200 {
		log.Printf("[TELEGRAM FAIL] Upload Status: %d | Body: %s", uploadResp.StatusCode, string(respBody))
	} else {
		log.Printf("[TELEGRAM SUCCESS] Image sent successfully!")
	}
}

// Fungsi Send Lainnya tetap sama
func (b *Bot) sendMessage(chatID int64, text string) {
	b.sendJSON("sendMessage", models.SendMessageRequest{
		ChatID: chatID, Text: text, ParseMode: "HTML",
	})
}
func (b *Bot) sendMessageWithKeyboard(chatID int64, text string, kb models.InlineKeyboardMarkup) {
	b.sendJSON("sendMessage", models.SendMessageRequest{
		ChatID: chatID, Text: text, ReplyMarkup: kb, ParseMode: "HTML",
	})
}
func (b *Bot) editMessageWithKeyboard(chatID int64, messageID int64, text string, kb models.InlineKeyboardMarkup) {
	b.sendJSON("editMessageText", models.EditMessageTextRequest{
		ChatID: chatID, MessageID: messageID, Text: text, ReplyMarkup: kb, ParseMode: "HTML",
	})
}

func (b *Bot) sendJSON(method string, data interface{}) {
	jsonData, _ := json.Marshal(data)
	resp, err := http.Post(fmt.Sprintf("%s/%s", b.APIURL, method), "application/json", bytes.NewBuffer(jsonData))
	
	if err != nil {
		log.Printf("[TELEGRAM ERROR] Network error calling %s: %v", method, err)
		return
	}
	defer resp.Body.Close()
}