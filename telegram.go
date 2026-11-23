package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Telegram struct {
	token string
	hc    *http.Client
}

func NewTelegram(token string) *Telegram {
	return &Telegram{token: token, hc: &http.Client{Timeout: 15 * time.Second}}
}

func (t *Telegram) Enabled() bool { return t.token != "" }

// Send matnni HTML formatida yuboradi.
func (t *Telegram) Send(ctx context.Context, chatID, text string) error {
	if t.token == "" {
		return errors.New("TELEGRAM_BOT_TOKEN berilmagan")
	}
	form := url.Values{
		"chat_id":                  {chatID},
		"text":                     {text},
		"parse_mode":               {"HTML"},
		"disable_web_page_preview": {"true"},
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.telegram.org/bot"+t.token+"/sendMessage", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.hc.Do(req)
	if err != nil {
		// xato matnida URL (va token) bo'lishi mumkin — yashiramiz
		return fmt.Errorf("telegram: %s", strings.ReplaceAll(err.Error(), t.token, "***"))
	}
	defer resp.Body.Close()

	var r struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return fmt.Errorf("telegram: HTTP %d", resp.StatusCode)
	}
	if !r.OK {
		return fmt.Errorf("telegram: %s", r.Description)
	}
	return nil
}
