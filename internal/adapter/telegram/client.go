// Package telegram — Telegram Bot API adapteri: xabar yuborish (usecase.Notifier)
// va kiruvchi yangilanishlarni olish (delivery/telegrambot uchun).
package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	token   string
	baseURL string
	hc      *http.Client

	mu       sync.Mutex
	username string
}

func NewClient(token string) *Client {
	return &Client{
		token:   token,
		baseURL: "https://api.telegram.org",
		hc:      &http.Client{Timeout: 60 * time.Second}, // long polling 30s dan uzunroq
	}
}

func (c *Client) Enabled() bool { return c.token != "" }

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
}

func (c *Client) call(ctx context.Context, method string, params url.Values, out any) error {
	if c.token == "" {
		return errors.New("TELEGRAM_BOT_TOKEN berilmagan")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/bot"+c.token+"/"+method, strings.NewReader(params.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.hc.Do(req)
	if err != nil {
		// xato matnida URL (demak token) bo'lishi mumkin — yashiramiz
		return fmt.Errorf("telegram: %s", strings.ReplaceAll(err.Error(), c.token, "***"))
	}
	defer resp.Body.Close()
	var r apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return fmt.Errorf("telegram: HTTP %d", resp.StatusCode)
	}
	if !r.OK {
		return fmt.Errorf("telegram: %s", r.Description)
	}
	if out != nil {
		return json.Unmarshal(r.Result, out)
	}
	return nil
}

// Username botning @username'i (getMe bir marta chaqiriladi va keshlanadi).
func (c *Client) Username(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.username != "" {
		return c.username, nil
	}
	var me struct {
		Username string `json:"username"`
	}
	if err := c.call(ctx, "getMe", url.Values{}, &me); err != nil {
		return "", err
	}
	c.username = me.Username
	return me.Username, nil
}

func (c *Client) SendMessage(ctx context.Context, chatID, text string, html bool) error {
	p := url.Values{"chat_id": {chatID}, "text": {text}, "disable_web_page_preview": {"true"}}
	if html {
		p.Set("parse_mode", "HTML")
	}
	return c.call(ctx, "sendMessage", p, nil)
}

// Update — botga kelgan matnli xabar (bizga faqat shu kerak).
type Update struct {
	ID       int64
	ChatID   string // shaxsiy chatda Telegram user ID bilan bir xil
	Username string
	Name     string
	Text     string
}

// GetUpdates long polling: offset'dan keyingi yangilanishlarni kutadi.
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeout time.Duration) ([]Update, error) {
	var raw []struct {
		UpdateID int64 `json:"update_id"`
		Message  *struct {
			Text string `json:"text"`
			From struct {
				ID        int64  `json:"id"`
				Username  string `json:"username"`
				FirstName string `json:"first_name"`
				LastName  string `json:"last_name"`
			} `json:"from"`
			Chat struct {
				ID   int64  `json:"id"`
				Type string `json:"type"`
			} `json:"chat"`
		} `json:"message"`
	}
	p := url.Values{
		"offset":          {strconv.FormatInt(offset, 10)},
		"timeout":         {strconv.Itoa(int(timeout.Seconds()))},
		"allowed_updates": {`["message"]`},
	}
	if err := c.call(ctx, "getUpdates", p, &raw); err != nil {
		return nil, err
	}
	out := make([]Update, 0, len(raw))
	for _, u := range raw {
		up := Update{ID: u.UpdateID}
		if u.Message != nil && u.Message.Chat.Type == "private" {
			up.ChatID = strconv.FormatInt(u.Message.Chat.ID, 10)
			up.Text = u.Message.Text
			up.Username = u.Message.From.Username
			up.Name = strings.TrimSpace(u.Message.From.FirstName + " " + u.Message.From.LastName)
		}
		out = append(out, up)
	}
	return out, nil
}
