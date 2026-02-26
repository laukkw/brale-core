package telegrambot

type update struct {
	UpdateID      int            `json:"update_id"`
	Message       *message       `json:"message,omitempty"`
	CallbackQuery *callbackQuery `json:"callback_query,omitempty"`
}

type message struct {
	MessageID int    `json:"message_id"`
	From      *user  `json:"from,omitempty"`
	Chat      *chat  `json:"chat,omitempty"`
	Text      string `json:"text,omitempty"`
}

type callbackQuery struct {
	ID      string   `json:"id"`
	From    *user    `json:"from,omitempty"`
	Message *message `json:"message,omitempty"`
	Data    string   `json:"data,omitempty"`
}

type user struct {
	ID        int64  `json:"id"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
}

type chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type,omitempty"`
}

type baseResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
	ErrorCode   int    `json:"error_code,omitempty"`
}

type updatesResponse struct {
	OK          bool     `json:"ok"`
	Result      []update `json:"result"`
	Description string   `json:"description,omitempty"`
	ErrorCode   int      `json:"error_code,omitempty"`
}

type sendMessageRequest struct {
	ChatID      int64  `json:"chat_id"`
	Text        string `json:"text"`
	ParseMode   string `json:"parse_mode,omitempty"`
	ReplyMarkup any    `json:"reply_markup,omitempty"`
}

type sendMessageResponse struct {
	OK          bool     `json:"ok"`
	Result      *message `json:"result,omitempty"`
	Description string   `json:"description,omitempty"`
	ErrorCode   int      `json:"error_code,omitempty"`
}

type answerCallbackRequest struct {
	CallbackQueryID string `json:"callback_query_id"`
}

type forceReply struct {
	ForceReply bool `json:"force_reply"`
}

type inlineKeyboard struct {
	Buttons [][]inlineButton
}

type inlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
}

type inlineKeyboardMarkup struct {
	InlineKeyboard [][]inlineButton `json:"inline_keyboard"`
}

func (k inlineKeyboard) toMarkup() inlineKeyboardMarkup {
	return inlineKeyboardMarkup{InlineKeyboard: k.Buttons}
}

type errorResponse struct {
	Code    string `json:"code"`
	Msg     string `json:"msg"`
	Details any    `json:"details,omitempty"`
}
