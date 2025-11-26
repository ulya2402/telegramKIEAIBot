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
	lang := b.DB.GetUserLanguage(userID)

	if text == "/start" {
		b.DB.SetUserState(userID, "IDLE", "")
		b.sendMessage(chatID, b.Localizer.Get(lang, "welcome"))
		return
	}

	if text == "/lang" {
		b.showLanguageMenu(chatID, 0, false, lang)
		return
	}

	if text == "/img" {
		b.showProviders(chatID, 0, false, lang)
		return
	}

	state := b.DB.GetUserState(userID)
	
	if state.State == "WAITING_IMAGE_UPLOAD" {
		b.sendMessage(chatID, b.Localizer.Get(lang, "upload_warn_wrong_mode"))
		return
	}

	if state.State == "WAITING_PROMPT" && state.SelectedModel != "" {
		b.processImageGeneration(chatID, userID, text, state, lang)
	} else {
		b.sendMessage(chatID, b.Localizer.Get(lang, "start_hint"))
	}
}

func (b *Bot) handlePhotoUpload(msg *models.TelegramMessage) {
	chatID := msg.Chat.ID
	userID := msg.From.ID
	state := b.DB.GetUserState(userID)
	lang := b.DB.GetUserLanguage(userID)

	if state.State != "WAITING_IMAGE_UPLOAD" {
		return 
	}

	bestPhoto := msg.Photo[len(msg.Photo)-1]
	fileURL, err := b.getFileDirectURL(bestPhoto.FileID)
	if err != nil {
		log.Printf("[ERROR] Failed to get file URL: %v", err)
		b.sendMessage(chatID, b.Localizer.Get(lang, "upload_fail_url"))
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
		b.sendMessage(chatID, b.Localizer.Get(lang, "upload_max_limit"))
		return
	}

	imageList = append(imageList, fileURL)
	b.DB.UpdateDraftOption(userID, "image_input", imageList)
	
	msgText := fmt.Sprintf(b.Localizer.Get(lang, "upload_received"), len(imageList))
	b.sendMessage(chatID, msgText)
}

func (b *Bot) handleCallback(cb *models.CallbackQuery) {
	parts := strings.SplitN(cb.Data, ":", 3)
	action := parts[0]
	chatID := cb.Message.Chat.ID
	messageID := cb.Message.MessageID
	userID := cb.From.ID
	lang := b.DB.GetUserLanguage(userID)

	http.Get(fmt.Sprintf("%s/answerCallbackQuery?callback_query_id=%s", b.APIURL, cb.ID))

	switch action {
	case "lang":
		if len(parts) > 1 {
			newLang := parts[1]
			b.DB.SetUserLanguage(userID, newLang)
			
			// Ambil pesan sukses dalam bahasa BARU
			successMsg := b.Localizer.Get(newLang, "menu_lang_success")
			
			// Edit pesan jadi konfirmasi sukses (hapus tombol)
			b.editMessageWithKeyboard(chatID, messageID, successMsg, models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}})
		}
	case "prov":
		if len(parts) > 1 {
			b.showModels(chatID, messageID, parts[1], lang)
		}
	
	case "model":
		if len(parts) > 1 {
			modelID := parts[1]
			b.DB.SetUserState(userID, "WAITING_PROMPT", modelID)
			
			b.DB.UpdateDraftOption(userID, "ratio", "1:1")
			b.DB.UpdateDraftOption(userID, "format", "png")
			b.DB.UpdateDraftOption(userID, "image_input", []string{}) 
			
			model := core.GetModelByID(modelID)
			for _, op := range model.SupportedOps {
				if op == "resolution" {
					b.DB.UpdateDraftOption(userID, "resolution", "1K")
				}
			}
			b.showModelDashboard(chatID, messageID, userID, modelID, lang)
		}

	case "dash":
		if len(parts) > 1 {
			modelID := parts[1]
			b.DB.SetUserState(userID, "WAITING_PROMPT", modelID)
			b.showModelDashboard(chatID, messageID, userID, modelID, lang)
		}

	case "set":
		if len(parts) > 1 {
			settingType := parts[1]
			if settingType == "image_input" {
				currentState := b.DB.GetUserState(userID)
				b.DB.SetUserState(userID, "WAITING_IMAGE_UPLOAD", currentState.SelectedModel) 
				
				kb := models.InlineKeyboardMarkup{
					InlineKeyboard: [][]models.InlineKeyboardButton{
						{{Text: b.Localizer.Get(lang, "btn_done"), CallbackData: "upload_done"}},
					},
				}
				b.editMessageWithKeyboard(chatID, messageID, b.Localizer.Get(lang, "upload_instruction"), kb)
			} else {
				b.showSettingOptions(chatID, messageID, userID, settingType, lang)
			}
		}

	case "upload_done":
		state := b.DB.GetUserState(userID)
		b.DB.SetUserState(userID, "WAITING_PROMPT", state.SelectedModel)
		b.showModelDashboard(chatID, messageID, userID, state.SelectedModel, lang)

	case "opt":
		if len(parts) > 2 {
			settingType := parts[1]
			value := parts[2]
			b.DB.UpdateDraftOption(userID, settingType, value)
			state := b.DB.GetUserState(userID)
			b.showModelDashboard(chatID, messageID, userID, state.SelectedModel, lang)
		}

	case "back_home":
		b.showProviders(chatID, messageID, true, lang)
	case "back_model":
		b.showModels(chatID, messageID, "google", lang)
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

func (b *Bot) showLanguageMenu(chatID int64, messageID int64, isEdit bool, lang string) {
	kb := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "🇺🇸 English", CallbackData: "lang:en"},
				{Text: "🇮🇩 Indonesia", CallbackData: "lang:id"},
			},
		},
	}
	text := b.Localizer.Get(lang, "menu_lang_title")
	if isEdit {
		b.editMessageWithKeyboard(chatID, messageID, text, kb)
	} else {
		b.sendMessageWithKeyboard(chatID, text, kb)
	}
}

func (b *Bot) showProviders(chatID int64, messageID int64, isEdit bool, lang string) {
	var rows [][]models.InlineKeyboardButton
	for _, p := range core.AI_REGISTRY {
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: p.Name, CallbackData: "prov:" + p.ID},
		})
	}
	kb := models.InlineKeyboardMarkup{InlineKeyboard: rows}
	text := b.Localizer.Get(lang, "select_provider")

	if isEdit {
		b.editMessageWithKeyboard(chatID, messageID, text, kb)
	} else {
		b.sendMessageWithKeyboard(chatID, text, kb)
	}
}

func (b *Bot) showModels(chatID int64, messageID int64, providerID string, lang string) {
	prov := core.GetProviderByID(providerID)
	if prov == nil {
		return
	}
	var rows [][]models.InlineKeyboardButton
	for _, m := range prov.Models {
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: m.Name, CallbackData: "model:" + m.ID},
		})
	}
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: b.Localizer.Get(lang, "btn_back"), CallbackData: "back_home"},
	})
	kb := models.InlineKeyboardMarkup{InlineKeyboard: rows}
	
	text := fmt.Sprintf(b.Localizer.Get(lang, "provider_msg"), prov.Name)
	b.editMessageWithKeyboard(chatID, messageID, text, kb)
}

func (b *Bot) showModelDashboard(chatID int64, messageID int64, userID int64, modelID string, lang string) {
	model := core.GetModelByID(modelID)
	state := b.DB.GetUserState(userID)
	opts := state.DraftOptions

	text := fmt.Sprintf(b.Localizer.Get(lang, "dash_model"), model.Name)
	text += fmt.Sprintf(b.Localizer.Get(lang, "dash_status"), b.Localizer.Get(lang, "dash_status_wait"))
	text += b.Localizer.Get(lang, "dash_settings")
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
			text += fmt.Sprintf(b.Localizer.Get(lang, "dash_files_count"), count)
		} else {
			text += fmt.Sprintf("• %-10s : %v\n", strings.Title(op), val)
		}
	}
	
	text += "</pre>\n"
	text += b.Localizer.Get(lang, "dash_footer")

	var rows [][]models.InlineKeyboardButton
	var row []models.InlineKeyboardButton

	for _, op := range model.SupportedOps {
		btnText := fmt.Sprintf(b.Localizer.Get(lang, "btn_set"), strings.Title(op))
		if op == "image_input" {
			btnText = b.Localizer.Get(lang, "btn_upload_img")
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
		{Text: b.Localizer.Get(lang, "btn_back_models"), CallbackData: "back_model"},
	})

	kb := models.InlineKeyboardMarkup{InlineKeyboard: rows}
	b.editMessageWithKeyboard(chatID, messageID, text, kb)
}

func (b *Bot) showSettingOptions(chatID int64, messageID int64, userID int64, settingType string, lang string) {
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
		{Text: b.Localizer.Get(lang, "btn_back"), CallbackData: "dash:" + model.ID},
	})

	kb := models.InlineKeyboardMarkup{InlineKeyboard: rows}
	text := fmt.Sprintf(b.Localizer.Get(lang, "select_option"), strings.Title(settingType))
	b.editMessageWithKeyboard(chatID, messageID, text, kb)
}

func (b *Bot) processImageGeneration(chatID int64, userID int64, prompt string, state database.UserState, lang string) {
	model := core.GetModelByID(state.SelectedModel)
	if model == nil {
		b.sendMessage(chatID, b.Localizer.Get(lang, "error_model_not_found"))
		return
	}
	
	startMsg := fmt.Sprintf(b.Localizer.Get(lang, "gen_start"), model.Name) 
	
	// FIX: Sekarang pemanggilannya jauh lebih simpel & bersih
	statusMsgID, _ := b.sendMessageReturnID(chatID, startMsg)
	
	go func() {
		taskID, err := b.KieClient.CreateTaskComplex(prompt, model.APIModelID, state.DraftOptions)
		if err != nil {
			log.Printf("API Error: %v", err)
			b.sendMessage(chatID, b.Localizer.Get(lang, "gen_fail_start"))
			return
		}
		b.pollTaskResult(chatID, taskID, model.ID, lang, prompt, statusMsgID, state.DraftOptions)
	}()
}

// FIX: Update parameter menjadi 7 buah
func (b *Bot) pollTaskResult(chatID int64, taskID string, modelID string, lang string, originalPrompt string, statusMsgID int64, options map[string]interface{}) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	timeout := time.After(3 * time.Minute)
	
	for {
		select {
		case <-timeout:
			// FIX: Hapus pesan status jika timeout
			if statusMsgID != 0 {
				b.deleteMessage(chatID, statusMsgID)
			}
			b.sendMessage(chatID, b.Localizer.Get(lang, "gen_timeout"))
			return
		case <-ticker.C:
			b.sendChatAction(chatID, "upload_photo")

			status, err := b.KieClient.GetTaskStatus(taskID, modelID)
			if err != nil {
				continue
			}
			
			if status.Data.State == "success" {
				var res models.KieResultJSON
				json.Unmarshal([]byte(status.Data.ResultJSON), &res)

				if len(res.ResultURLs) > 0 {
					imgURL := res.ResultURLs[0]
					
					// FIX: Hapus pesan status "Waiting..."
					if statusMsgID != 0 {
						b.deleteMessage(chatID, statusMsgID)
					}

					// Format Caption
					displayPrompt := originalPrompt
					if len(displayPrompt) > 300 {
						displayPrompt = displayPrompt[:300] + "..."
					}
					
					ratio := "1:1"
					if r, ok := options["ratio"].(string); ok {
						ratio = r
					}
					
					modelName := "Unknown Model"
					modelObj := core.GetModelByID(modelID)
					if modelObj != nil {
						modelName = modelObj.Name
					}

					caption := fmt.Sprintf(b.Localizer.Get(lang, "gen_caption"), modelName, ratio, displayPrompt)

					b.sendPhoto(chatID, imgURL, caption, lang)
				} else {
					if statusMsgID != 0 {
						b.deleteMessage(chatID, statusMsgID)
					}
					b.sendMessage(chatID, b.Localizer.Get(lang, "gen_result_empty"))
				}
				return
			} else if status.Data.State == "fail" {
				if statusMsgID != 0 {
					b.deleteMessage(chatID, statusMsgID)
				}
				failMsg := fmt.Sprintf(b.Localizer.Get(lang, "gen_fail"), status.Data.FailMsg)
				b.sendMessage(chatID, failMsg)
				return
			}
		}
	}
}

func (b *Bot) sendPhoto(chatID int64, photoURL, caption, lang string) {	
	resp, err := http.Get(photoURL)
	if err != nil {
		log.Printf("Download failed: %v", err)
		b.sendMessage(chatID, b.Localizer.Get(lang, "err_download"))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b.sendMessage(chatID, b.Localizer.Get(lang, "err_server"))
		return
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("chat_id", fmt.Sprintf("%d", chatID))
	writer.WriteField("caption", caption)

	writer.WriteField("parse_mode", "HTML")
	
	part, err := writer.CreateFormFile("photo", "image.png")
	if err != nil {
		return
	}
	io.Copy(part, resp.Body)
	writer.Close()

	uploadReq, err := http.NewRequest("POST", fmt.Sprintf("%s/sendPhoto", b.APIURL), body)
	if err != nil {
		return
	}
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 60 * time.Second}
	uploadResp, err := client.Do(uploadReq)
	if err != nil {
		log.Printf("Upload to Telegram failed: %v", err)
		b.sendMessage(chatID, b.Localizer.Get(lang, "err_send_tele"))
		return
	}
	defer uploadResp.Body.Close()
}

func (b *Bot) sendMessage(chatID int64, text string) {
	b.sendJSON("sendMessage", models.SendMessageRequest{
		ChatID: chatID, Text: text, ParseMode: "HTML",
	})
}

// Ubah return type menjadi (int64, error)
func (b *Bot) sendMessageReturnID(chatID int64, text string) (int64, error) {
	jsonData, _ := json.Marshal(models.SendMessageRequest{
		ChatID: chatID, Text: text, ParseMode: "HTML",
	})
	resp, err := http.Post(fmt.Sprintf("%s/sendMessage", b.APIURL), "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	
	// Kita buat struct khusus di sini untuk menangkap respon sendMessage
	// karena formatnya beda dengan getUpdates
	var response struct {
		Ok     bool                    `json:"ok"`
		Result *models.TelegramMessage `json:"result"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return 0, err
	}

	if !response.Ok || response.Result == nil {
		return 0, fmt.Errorf("failed to send message")
	}

	return response.Result.MessageID, nil
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
		log.Printf("Network error: %v", err)
		return
	}
	defer resp.Body.Close()
}

func (b *Bot) sendChatAction(chatID int64, action string) {
	req := models.SendChatActionRequest{ChatID: chatID, Action: action}
	b.sendJSON("sendChatAction", req)
}

func (b *Bot) deleteMessage(chatID int64, messageID int64) {
	req := models.DeleteMessageRequest{ChatID: chatID, MessageID: messageID}
	b.sendJSON("deleteMessage", req)
}