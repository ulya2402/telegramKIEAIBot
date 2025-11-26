package models

type TelegramUpdate struct {
	UpdateID      int64            `json:"update_id"`
	Message       *TelegramMessage `json:"message"`
	CallbackQuery *CallbackQuery   `json:"callback_query"`
}

type TelegramMessage struct {
	MessageID int64       `json:"message_id"`
	From      *User       `json:"from"`
	Chat      *Chat       `json:"chat"`
	Text      string      `json:"text"`
	Photo     []PhotoSize `json:"photo"` // Tambahan Baru
}

type PhotoSize struct {
	FileID   string `json:"file_id"`
	FileSize int    `json:"file_size"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

type CallbackQuery struct {
	ID      string           `json:"id"`
	From    *User            `json:"from"`
	Message *TelegramMessage `json:"message"`
	Data    string           `json:"data"`
}

type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
	Language  string `json:"language_code"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type TelegramResponse struct {
	Ok     bool             `json:"ok"`
	Result []TelegramUpdate `json:"result"`
}

type SendMessageRequest struct {
	ChatID      int64       `json:"chat_id"`
	Text        string      `json:"text"`
	ParseMode   string      `json:"parse_mode,omitempty"`
	ReplyMarkup interface{} `json:"reply_markup,omitempty"`
}

type EditMessageTextRequest struct {
	ChatID      int64       `json:"chat_id"`
	MessageID   int64       `json:"message_id"`
	Text        string      `json:"text"`
	ParseMode   string      `json:"parse_mode,omitempty"`
	ReplyMarkup interface{} `json:"reply_markup,omitempty"`
}

type SendPhotoRequest struct {
	ChatID  int64  `json:"chat_id"`
	Photo   string `json:"photo"`
	Caption string `json:"caption,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}