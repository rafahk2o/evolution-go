package chatwoot

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	logger_wrapper "github.com/EvolutionAPI/evolution-go/pkg/logger"
)

type Client struct {
	httpClient    *http.Client
	loggerWrapper *logger_wrapper.LoggerManager
}

type IncomingMessage struct {
	SourceID string
	Phone    string
	Name     string
	Content  string
	EchoID   string
}

type conversationResponse struct {
	ID     int    `json:"id"`
	Status string `json:"status"`
}

func NewClient(loggerWrapper *logger_wrapper.LoggerManager) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
		loggerWrapper: loggerWrapper,
	}
}

func (c *Client) Enabled(instance *instance_model.Instance) bool {
	return c != nil &&
		instance != nil &&
		instance.ChatwootEnabled &&
		instance.ChatwootURL != "" &&
		instance.ChatwootInboxIdentifier != ""
}

func (c *Client) ForwardEvolutionEvent(instance *instance_model.Instance, payload []byte) error {
	if !c.Enabled(instance) {
		return nil
	}

	settings := settingsFromInstance(instance)
	message, err := extractIncomingMessage(payload, settings.EnableGroups)
	if err != nil {
		return err
	}
	if message == nil {
		return nil
	}

	if err := c.upsertContact(settings, message); err != nil {
		return err
	}

	conversationID, err := c.findOrCreateConversation(settings, message.SourceID)
	if err != nil {
		return err
	}

	if err := c.createIncomingMessage(settings, message.SourceID, conversationID, message); err != nil {
		return err
	}

	c.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Chatwoot incoming message synced - source: %s conversation: %d", instance.Id, message.SourceID, conversationID)
	return nil
}

func (c *Client) upsertContact(settings *instance_model.ChatwootSettings, message *IncomingMessage) error {
	body := map[string]interface{}{
		"source_id":    message.SourceID,
		"identifier":   message.SourceID,
		"name":         message.Name,
		"phone_number": message.Phone,
		"custom_attributes": map[string]string{
			"source": "evolution-go",
		},
	}
	c.addIdentifierHash(settings, body, message.SourceID)

	path := fmt.Sprintf("/public/api/v1/inboxes/%s/contacts", url.PathEscape(settings.InboxIdentifier))
	_, err := c.doJSON(settings, http.MethodPost, path, body)
	return err
}

func (c *Client) findOrCreateConversation(settings *instance_model.ChatwootSettings, sourceID string) (int, error) {
	path := fmt.Sprintf(
		"/public/api/v1/inboxes/%s/contacts/%s/conversations",
		url.PathEscape(settings.InboxIdentifier),
		url.PathEscape(sourceID),
	)

	body, err := c.doJSON(settings, http.MethodGet, path, nil)
	if err != nil {
		return 0, err
	}

	var conversations []conversationResponse
	if err := json.Unmarshal(body, &conversations); err != nil {
		return 0, err
	}

	for i := len(conversations) - 1; i >= 0; i-- {
		if conversations[i].Status != "resolved" {
			return conversations[i].ID, nil
		}
	}

	body, err = c.doJSON(settings, http.MethodPost, path, map[string]interface{}{})
	if err != nil {
		return 0, err
	}

	var created conversationResponse
	if err := json.Unmarshal(body, &created); err != nil {
		return 0, err
	}
	if created.ID == 0 {
		return 0, errors.New("chatwoot returned empty conversation id")
	}
	return created.ID, nil
}

func (c *Client) createIncomingMessage(settings *instance_model.ChatwootSettings, sourceID string, conversationID int, message *IncomingMessage) error {
	path := fmt.Sprintf(
		"/public/api/v1/inboxes/%s/contacts/%s/conversations/%d/messages",
		url.PathEscape(settings.InboxIdentifier),
		url.PathEscape(sourceID),
		conversationID,
	)

	body := map[string]string{
		"content": message.Content,
		"echo_id": message.EchoID,
	}
	_, err := c.doJSON(settings, http.MethodPost, path, body)
	return err
}

func (c *Client) doJSON(settings *instance_model.ChatwootSettings, method, path string, body interface{}) ([]byte, error) {
	base := strings.TrimRight(settings.URL, "/")
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, base+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if settings.AccountToken != "" {
		req.Header.Set("api_access_token", settings.AccountToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("chatwoot %s %s failed with status %s: %s", method, path, resp.Status, string(responseBody))
	}
	return responseBody, nil
}

func (c *Client) addIdentifierHash(settings *instance_model.ChatwootSettings, body map[string]interface{}, identifier string) {
	if settings.HMACToken == "" || identifier == "" {
		return
	}
	hash := hmac.New(sha256.New, []byte(settings.HMACToken))
	hash.Write([]byte(identifier))
	body["identifier_hash"] = hex.EncodeToString(hash.Sum(nil))
}

func settingsFromInstance(instance *instance_model.Instance) *instance_model.ChatwootSettings {
	return &instance_model.ChatwootSettings{
		Enabled:         instance.ChatwootEnabled,
		URL:             instance.ChatwootURL,
		AccountID:       instance.ChatwootAccountID,
		AccountToken:    instance.ChatwootAccountToken,
		InboxID:         instance.ChatwootInboxID,
		InboxIdentifier: instance.ChatwootInboxIdentifier,
		WebhookToken:    instance.ChatwootWebhookToken,
		HMACToken:       instance.ChatwootHMACToken,
		EnableGroups:    instance.ChatwootEnableGroups,
	}
}

func extractIncomingMessage(payload []byte, enableGroups bool) (*IncomingMessage, error) {
	var event map[string]interface{}
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, err
	}

	eventName := strings.ToLower(stringValue(event["event"]))
	if eventName != "message" && eventName != "message.received" && eventName != "message_received" {
		return nil, nil
	}

	data, _ := event["data"].(map[string]interface{})
	if data == nil {
		return nil, nil
	}

	info, _ := data["Info"].(map[string]interface{})
	if info == nil {
		info, _ = data["key"].(map[string]interface{})
	}

	if boolValue(firstValue(info, "IsFromMe", "fromMe")) {
		return nil, nil
	}

	chat := firstString(info, "Chat", "remoteJid")
	sender := firstString(info, "Sender", "Participant", "participant")
	if sender == "" {
		sender = chat
	}

	if strings.Contains(chat, "@g.us") && !enableGroups {
		return nil, nil
	}

	sourceJID := sender
	if sourceJID == "" {
		sourceJID = chat
	}

	phoneDigits := phoneFromJID(sourceJID)
	if phoneDigits == "" {
		return nil, nil
	}

	content := extractContent(data)
	if strings.TrimSpace(content) == "" {
		return nil, nil
	}

	name := firstString(data, "PushName", "pushName")
	if name == "" {
		name = phoneDigits
	}

	echoID := firstString(info, "ID", "id")

	return &IncomingMessage{
		SourceID: phoneDigits,
		Phone:    "+" + phoneDigits,
		Name:     name,
		Content:  content,
		EchoID:   echoID,
	}, nil
}

func extractContent(data map[string]interface{}) string {
	message, _ := data["Message"].(map[string]interface{})
	if message == nil {
		message, _ = data["message"].(map[string]interface{})
	}
	if message == nil {
		return ""
	}

	if text := stringValue(firstValue(message, "conversation")); text != "" {
		return text
	}
	if mediaURL := stringValue(message["mediaUrl"]); mediaURL != "" {
		if mimetype := stringValue(message["mimetype"]); mimetype != "" {
			return "[" + mimetype + "] " + mediaURL
		}
		return mediaURL
	}

	messageTypes := []string{
		"extendedTextMessage",
		"imageMessage",
		"videoMessage",
		"documentMessage",
		"audioMessage",
		"stickerMessage",
		"buttonsResponseMessage",
		"templateButtonReplyMessage",
		"listResponseMessage",
	}

	for _, messageType := range messageTypes {
		part, _ := message[messageType].(map[string]interface{})
		if part == nil {
			continue
		}
		for _, key := range []string{"text", "caption", "selectedDisplayText", "title", "description"} {
			if value := stringValue(part[key]); value != "" {
				return value
			}
		}
		if mediaURL := stringValue(part["mediaUrl"]); mediaURL != "" {
			return "[" + strings.TrimSuffix(messageType, "Message") + "] " + mediaURL
		}
	}

	for _, mediaType := range []string{"imageMessage", "videoMessage", "documentMessage", "audioMessage", "stickerMessage"} {
		part, _ := message[mediaType].(map[string]interface{})
		if part == nil {
			continue
		}
		if mediaURL := stringValue(firstValue(part, "mediaUrl", "url", "directPath")); mediaURL != "" {
			return "[" + strings.TrimSuffix(mediaType, "Message") + "] " + mediaURL
		}
		return "[" + strings.TrimSuffix(mediaType, "Message") + "]"
	}

	return ""
}

func phoneFromJID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if at := strings.Index(value, "@"); at >= 0 {
		value = value[:at]
	}
	if colon := strings.Index(value, ":"); colon >= 0 {
		value = value[:colon]
	}

	re := regexp.MustCompile(`\D+`)
	return re.ReplaceAllString(value, "")
}

func firstString(data map[string]interface{}, keys ...string) string {
	return stringValue(firstValue(data, keys...))
}

func firstValue(data map[string]interface{}, keys ...string) interface{} {
	if data == nil {
		return nil
	}
	for _, key := range keys {
		if value, ok := data[key]; ok {
			return value
		}
	}
	return nil
}

func stringValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case float64:
		if typed == 0 {
			return ""
		}
		return strconv.FormatInt(int64(typed), 10)
	default:
		return ""
	}
}

func boolValue(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(typed, "true")
	default:
		return false
	}
}
