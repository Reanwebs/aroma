package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var nonDigits = regexp.MustCompile(`\D`)

func (a *API) sendWhatsApp(ctx context.Context, phone, message string) error {
	if !a.cfg.WhatsAppEnabled {
		return fmt.Errorf("WhatsApp sending is disabled in server settings")
	}
	endpoint := a.cfg.WhatsAppAPIURL
	if endpoint == "" && a.cfg.WhatsAppPhoneNumberID != "" {
		endpoint = "https://graph.facebook.com/v25.0/" + a.cfg.WhatsAppPhoneNumberID + "/messages"
	}
	if endpoint == "" || a.cfg.WhatsAppAccessToken == "" {
		return fmt.Errorf("WhatsApp API URL or access token is not configured")
	}
	to := nonDigits.ReplaceAllString(phone, "")
	if len(to) == 10 {
		to = "91" + to
	}
	if len(to) < 8 || len(to) > 15 {
		return fmt.Errorf("customer does not have a valid WhatsApp number")
	}
	message = strings.TrimSpace(message)
	if message == "" || len([]rune(message)) > 4096 {
		return fmt.Errorf("WhatsApp message must contain 1 to 4096 characters")
	}
	body, err := json.Marshal(map[string]any{
		"messaging_product": "whatsapp", "recipient_type": "individual", "to": to,
		"type": "text", "text": map[string]any{"preview_url": false, "body": message},
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("invalid WhatsApp API URL")
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.WhatsAppAccessToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach the WhatsApp API: %w", err)
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var meta struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(responseBody, &meta) == nil && meta.Error.Message != "" {
			return fmt.Errorf("WhatsApp rejected the message: %s", meta.Error.Message)
		}
		return fmt.Errorf("WhatsApp API returned HTTP %d", resp.StatusCode)
	}
	return nil
}
