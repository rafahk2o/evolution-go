package chatwoot

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	logger_wrapper "github.com/EvolutionAPI/evolution-go/pkg/logger"
	"github.com/patrickmn/go-cache"
)

type Client struct {
	httpClient    *http.Client
	mediaClient   *http.Client
	loggerWrapper *logger_wrapper.LoggerManager
	outgoing      *cache.Cache
}

type Attachment struct {
	Data     []byte
	Filename string
	Mimetype string
}

type IncomingMessage struct {
	SourceID   string
	Phone      string
	Name       string
	Content    string
	EchoID     string
	Attachment *Attachment
}

type mediaInfo struct {
	URL      string
	Base64   string
	Mimetype string
	Filename string
	Kind     string
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
		mediaClient: &http.Client{
			Timeout: 90 * time.Second,
		},
		loggerWrapper: loggerWrapper,
		outgoing:      cache.New(24*time.Hour, 1*time.Hour),
	}
}

// Logger expõe o logger interno para uso por handlers que possuem o Client.
func (c *Client) Logger(instanceID string) *logger_wrapper.Logger {
	return c.loggerWrapper.GetLogger(instanceID)
}

// RegisterOutgoing memoriza o mapeamento entre o ID da mensagem do Chatwoot
// e a conversa onde ela foi enviada. Usamos isso para atualizar o status
// (sent/delivered/read) quando o WhatsApp emite recibos.
func (c *Client) RegisterOutgoing(chatwootMsgID, conversationID string) {
	if c == nil || c.outgoing == nil {
		return
	}
	chatwootMsgID = strings.TrimSpace(chatwootMsgID)
	conversationID = strings.TrimSpace(conversationID)
	if chatwootMsgID == "" || conversationID == "" {
		return
	}
	c.outgoing.Set(chatwootMsgID, conversationID, cache.DefaultExpiration)
}

func (c *Client) lookupOutgoing(chatwootMsgID string) (string, bool) {
	if c == nil || c.outgoing == nil {
		return "", false
	}
	v, ok := c.outgoing.Get(strings.TrimSpace(chatwootMsgID))
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
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

	var event map[string]interface{}
	if err := json.Unmarshal(payload, &event); err != nil {
		return err
	}

	switch strings.ToLower(stringValue(event["event"])) {
	case "receipt":
		return c.handleReceipt(instance, event)
	}

	settings := settingsFromInstance(instance)
	message, err := c.extractIncomingMessage(instance, payload, settings.EnableGroups)
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

	c.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Chatwoot incoming message synced - source: %s conversation: %d hasAttachment=%v", instance.Id, message.SourceID, conversationID, message.Attachment != nil)
	return nil
}

// handleReceipt traduz recibos do WhatsApp (Delivered/Read) em atualizações
// de status no Chatwoot para mensagens que o agente enviou.
func (c *Client) handleReceipt(instance *instance_model.Instance, event map[string]interface{}) error {
	logger := c.loggerWrapper.GetLogger(instance.Id)

	state := strings.ToLower(stringValue(event["state"]))
	var status string
	switch state {
	case "read":
		status = "read"
	case "delivered":
		status = "delivered"
	default:
		logger.LogInfo("[%s] chatwoot receipt: ignoring state=%q", instance.Id, state)
		return nil
	}

	data, _ := event["data"].(map[string]interface{})
	if data == nil {
		logger.LogWarn("[%s] chatwoot receipt: missing data field", instance.Id)
		return nil
	}

	ids := extractMessageIDs(data)
	if len(ids) == 0 {
		logger.LogWarn("[%s] chatwoot receipt: no message IDs found in payload (state=%s)", instance.Id, state)
		return nil
	}

	logger.LogInfo("[%s] chatwoot receipt received: state=%s ids=%v", instance.Id, state, ids)

	matched := 0
	for _, id := range ids {
		if !strings.HasPrefix(strings.ToLower(id), "chatwoot_") {
			continue
		}
		chatwootMsgID := id[len("chatwoot_"):]
		conversationID, ok := c.lookupOutgoing(chatwootMsgID)
		if !ok {
			logger.LogWarn("[%s] chatwoot receipt: no conversation cached for msg=%s — agent reply mapping missing", instance.Id, chatwootMsgID)
			continue
		}
		matched++
		if err := c.updateChatwootMessageStatus(instance, conversationID, chatwootMsgID, status); err != nil {
			logger.LogError("[%s] chatwoot status update failed (msg=%s conv=%s status=%s): %v", instance.Id, chatwootMsgID, conversationID, status, err)
		} else {
			logger.LogInfo("[%s] chatwoot status updated (msg=%s conv=%s status=%s)", instance.Id, chatwootMsgID, conversationID, status)
		}
	}
	if matched == 0 {
		logger.LogInfo("[%s] chatwoot receipt: no chatwoot-prefixed IDs in this batch", instance.Id)
	}
	return nil
}

func (c *Client) updateChatwootMessageStatus(instance *instance_model.Instance, conversationID, messageID, status string) error {
	if instance.ChatwootURL == "" || instance.ChatwootAccountID == "" || instance.ChatwootAccountToken == "" {
		return errors.New("chatwoot account credentials missing")
	}

	endpoint := fmt.Sprintf("%s/api/v1/accounts/%s/conversations/%s/messages/%s",
		strings.TrimRight(instance.ChatwootURL, "/"),
		url.PathEscape(instance.ChatwootAccountID),
		url.PathEscape(conversationID),
		url.PathEscape(messageID),
	)

	body, _ := json.Marshal(map[string]string{"status": status})
	req, err := http.NewRequest(http.MethodPatch, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api_access_token", instance.ChatwootAccountToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("chatwoot PATCH %s: %s — %s", endpoint, resp.Status, string(respBody))
	}
	return nil
}

func extractMessageIDs(data map[string]interface{}) []string {
	keys := []string{"MessageIDs", "MessageIds", "messageIds", "message_ids", "messageIDs", "ids"}
	for _, key := range keys {
		raw, ok := data[key].([]interface{})
		if !ok || len(raw) == 0 {
			continue
		}
		out := make([]string, 0, len(raw))
		for _, v := range raw {
			if s := stringValue(v); s != "" {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
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

	if message.Attachment != nil {
		responseBody, err := c.postMultipartMessage(settings, path, message)
		if err == nil {
			c.scheduleAttachmentRefresh(settings, conversationID, responseBody)
		}
		return err
	}

	body := map[string]string{
		"content": message.Content,
		"echo_id": message.EchoID,
	}
	_, err := c.doJSON(settings, http.MethodPost, path, body)
	return err
}

func (c *Client) postMultipartMessage(settings *instance_model.ChatwootSettings, path string, message *IncomingMessage) ([]byte, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	if message.Content != "" {
		if err := mw.WriteField("content", message.Content); err != nil {
			return nil, err
		}
	}
	if message.EchoID != "" {
		_ = mw.WriteField("echo_id", message.EchoID)
	}

	header := make(textproto.MIMEHeader)
	filename := message.Attachment.Filename
	if filename == "" {
		filename = "file"
	}
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="attachments[]"; filename="%s"`, escapeMultipartQuotes(filename)))
	if message.Attachment.Mimetype != "" {
		header.Set("Content-Type", message.Attachment.Mimetype)
	} else {
		header.Set("Content-Type", "application/octet-stream")
	}
	part, err := mw.CreatePart(header)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(message.Attachment.Data); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	base := strings.TrimRight(settings.URL, "/")
	req, err := http.NewRequest(http.MethodPost, base+path, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if settings.AccountToken != "" {
		req.Header.Set("api_access_token", settings.AccountToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("chatwoot multipart message failed with status %s: %s", resp.Status, string(respBody))
	}
	return respBody, nil
}

func (c *Client) scheduleAttachmentRefresh(settings *instance_model.ChatwootSettings, conversationID int, responseBody []byte) {
	if settings == nil || settings.URL == "" || settings.AccountID == "" || settings.AccountToken == "" {
		return
	}
	messageID := extractChatwootMessageID(responseBody)
	if messageID == "" {
		return
	}

	go func() {
		time.Sleep(1500 * time.Millisecond)
		if err := c.refreshChatwootAttachmentMessage(settings, conversationID, messageID); err != nil {
			c.loggerWrapper.GetLogger("").LogWarn("chatwoot attachment refresh failed: %v", err)
		}
	}()
}

func (c *Client) refreshChatwootAttachmentMessage(settings *instance_model.ChatwootSettings, conversationID int, messageID string) error {
	endpoint := fmt.Sprintf("%s/api/v1/accounts/%s/conversations/%d/messages/%s",
		strings.TrimRight(settings.URL, "/"),
		url.PathEscape(settings.AccountID),
		conversationID,
		url.PathEscape(messageID),
	)

	body, _ := json.Marshal(map[string]interface{}{
		"content_attributes": map[string]interface{}{
			"evolution_attachment_refresh_at": time.Now().UnixNano(),
		},
	})
	req, err := http.NewRequest(http.MethodPatch, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api_access_token", settings.AccountToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("chatwoot attachment refresh PATCH %s: %s - %s", endpoint, resp.Status, string(respBody))
	}
	return nil
}

func extractChatwootMessageID(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ""
	}
	return firstNestedString(parsed,
		[]string{"id"},
		[]string{"ID"},
		[]string{"message", "id"},
		[]string{"message", "ID"},
		[]string{"payload", "id"},
		[]string{"payload", "ID"},
		[]string{"data", "id"},
		[]string{"data", "ID"},
	)
}

func firstNestedString(data map[string]interface{}, paths ...[]string) string {
	for _, path := range paths {
		var current interface{} = data
		for _, key := range path {
			m, ok := current.(map[string]interface{})
			if !ok {
				current = nil
				break
			}
			current = m[key]
		}
		if value := stringValue(current); value != "" {
			return value
		}
	}
	return ""
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

func (c *Client) extractIncomingMessage(instance *instance_model.Instance, payload []byte, enableGroups bool) (*IncomingMessage, error) {
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
	media := extractMediaInfo(data, info)

	var attachment *Attachment
	if media != nil {
		attachment = c.resolveAttachment(instance, media)
	}

	if attachment == nil && strings.TrimSpace(content) == "" {
		if contacts := extractContacts(data); len(contacts) > 0 {
			content = formatContactsText(contacts)
			attachment = vcardsAttachment(contacts)
		}
	}

	if strings.TrimSpace(content) == "" && attachment == nil {
		if hasMediaSubtype(data) {
			content = "[Mídia recebida, mas não foi possível baixá-la]"
		} else {
			return nil, nil
		}
	}

	name := firstString(data, "PushName", "pushName")
	if name == "" {
		name = phoneDigits
	}

	echoID := firstString(info, "ID", "id")

	return &IncomingMessage{
		SourceID:   phoneDigits,
		Phone:      "+" + phoneDigits,
		Name:       name,
		Content:    content,
		EchoID:     echoID,
		Attachment: attachment,
	}, nil
}

func (c *Client) resolveAttachment(instance *instance_model.Instance, info *mediaInfo) *Attachment {
	if info == nil {
		return nil
	}
	var (
		data []byte
		err  error
	)
	switch {
	case info.Base64 != "":
		data, err = base64.StdEncoding.DecodeString(info.Base64)
	case info.URL != "":
		data, err = c.downloadMedia(info.URL)
	default:
		return nil
	}
	if err != nil {
		instanceID := ""
		if instance != nil {
			instanceID = instance.Id
		}
		c.loggerWrapper.GetLogger(instanceID).LogError("[%s] chatwoot: failed to resolve attachment: %v", instanceID, err)
		return nil
	}
	if len(data) == 0 {
		return nil
	}
	return &Attachment{
		Data:     data,
		Filename: info.Filename,
		Mimetype: info.Mimetype,
	}
}

func (c *Client) downloadMedia(rawURL string) ([]byte, error) {
	body, _, err := c.DownloadMedia(rawURL)
	return body, err
}

func (c *Client) DownloadChatwootMedia(rawURL, accountToken string) ([]byte, string, error) {
	headers := map[string]string{}
	if strings.TrimSpace(accountToken) != "" {
		headers["api_access_token"] = accountToken
	}
	return c.downloadMediaWithHeaders(rawURL, headers)
}

// DownloadMedia faz GET na URL com User-Agent e timeout estendido,
// retorna os bytes e o Content-Type da resposta.
// Usado tanto pelo fluxo WA→Chatwoot (mediaUrl do Minio) quanto pelo
// fluxo Chatwoot→WA (data_url da ActiveStorage do Chatwoot).
func (c *Client) DownloadMedia(rawURL string) ([]byte, string, error) {
	return c.downloadMediaWithHeaders(rawURL, nil)
}

func (c *Client) downloadMediaWithHeaders(rawURL string, headers map[string]string) ([]byte, string, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "evolution-go/1.0")
	req.Header.Set("Accept", "*/*")
	for key, value := range headers {
		if strings.TrimSpace(value) != "" {
			req.Header.Set(key, value)
		}
	}

	resp, err := c.mediaClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("download %s failed: %s", rawURL, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	ct := resp.Header.Get("Content-Type")
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	return body, ct, nil
}

func extractContent(data map[string]interface{}) string {
	message := messageMapOf(data)
	if message == nil {
		return ""
	}

	if text := stringValue(message["conversation"]); text != "" {
		return text
	}

	if ext, ok := message["extendedTextMessage"].(map[string]interface{}); ok && ext != nil {
		if text := stringValue(firstValue(ext, "text", "title", "description")); text != "" {
			return text
		}
	}

	for _, mt := range []string{"imageMessage", "videoMessage", "documentMessage"} {
		part, ok := message[mt].(map[string]interface{})
		if !ok || part == nil {
			continue
		}
		if caption := stringValue(part["caption"]); caption != "" {
			return caption
		}
	}

	for _, mt := range []string{"buttonsResponseMessage", "templateButtonReplyMessage"} {
		part, ok := message[mt].(map[string]interface{})
		if !ok || part == nil {
			continue
		}
		if v := stringValue(firstValue(part, "selectedDisplayText", "title")); v != "" {
			return v
		}
	}

	if part, ok := message["listResponseMessage"].(map[string]interface{}); ok && part != nil {
		if v := stringValue(firstValue(part, "title", "description")); v != "" {
			return v
		}
	}

	if loc := extractLocationText(message); loc != "" {
		return loc
	}

	return ""
}

func extractLocationText(message map[string]interface{}) string {
	for _, mt := range []string{"locationMessage", "liveLocationMessage"} {
		part, ok := message[mt].(map[string]interface{})
		if !ok || part == nil {
			continue
		}
		lat, latOk := floatValue(firstValue(part, "degreesLatitude", "DegreesLatitude"))
		lng, lngOk := floatValue(firstValue(part, "degreesLongitude", "DegreesLongitude"))
		if !latOk || !lngOk || (lat == 0 && lng == 0) {
			continue
		}

		var b strings.Builder
		if mt == "liveLocationMessage" {
			b.WriteString("📍 Localização ao vivo")
		} else {
			b.WriteString("📍 Localização")
		}

		if name := strings.TrimSpace(firstString(part, "name", "Name")); name != "" {
			b.WriteString("\n")
			b.WriteString(name)
		}
		if address := strings.TrimSpace(firstString(part, "address", "Address")); address != "" {
			b.WriteString("\n")
			b.WriteString(address)
		}
		if caption := strings.TrimSpace(firstString(part, "caption", "Caption")); caption != "" {
			b.WriteString("\n")
			b.WriteString(caption)
		}

		b.WriteString(fmt.Sprintf("\nhttps://www.google.com/maps?q=%.6f,%.6f", lat, lng))
		return b.String()
	}
	return ""
}

func floatValue(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

func extractMediaInfo(data map[string]interface{}, info map[string]interface{}) *mediaInfo {
	message := messageMapOf(data)
	if message == nil {
		return nil
	}

	mediaURL := stringValue(message["mediaUrl"])
	base64Data := stringValue(message["base64"])
	if mediaURL == "" && base64Data == "" {
		return nil
	}

	mimetype := stringValue(message["mimetype"])
	var fileName, kind string

	for _, mt := range []string{"imageMessage", "videoMessage", "audioMessage", "documentMessage", "stickerMessage"} {
		part, ok := message[mt].(map[string]interface{})
		if !ok || part == nil {
			continue
		}
		if mimetype == "" {
			mimetype = stringValue(part["mimetype"])
		}
		if fn := stringValue(part["fileName"]); fn != "" {
			fileName = fn
		}
		kind = strings.TrimSuffix(mt, "Message")
		break
	}

	if fileName == "" {
		id := stringValue(firstValue(info, "ID", "id"))
		if id == "" {
			id = "media"
		}
		fileName = id + extensionFromMimetype(mimetype, kind)
	}
	if mimetype == "" {
		mimetype = "application/octet-stream"
	}

	return &mediaInfo{
		URL:      mediaURL,
		Base64:   base64Data,
		Mimetype: mimetype,
		Filename: fileName,
		Kind:     kind,
	}
}

func hasMediaSubtype(data map[string]interface{}) bool {
	message := messageMapOf(data)
	if message == nil {
		return false
	}
	for _, mt := range []string{"imageMessage", "videoMessage", "audioMessage", "documentMessage", "stickerMessage"} {
		if part, ok := message[mt].(map[string]interface{}); ok && part != nil {
			return true
		}
	}
	return false
}

type contactInfo struct {
	DisplayName string
	Phone       string
	Vcard       string
}

var (
	vcardWaidRegex = regexp.MustCompile(`(?i)waid=([0-9]+)`)
	vcardTelRegex  = regexp.MustCompile(`(?im)^TEL[^:\r\n]*:([^\r\n]+)`)
)

func extractContacts(data map[string]interface{}) []contactInfo {
	message := messageMapOf(data)
	if message == nil {
		return nil
	}

	var out []contactInfo

	if cm, ok := message["contactMessage"].(map[string]interface{}); ok && cm != nil {
		if c := buildContactInfo(cm); c != nil {
			out = append(out, *c)
		}
	}

	if cam, ok := message["contactsArrayMessage"].(map[string]interface{}); ok && cam != nil {
		items, _ := firstValue(cam, "contacts", "Contacts").([]interface{})
		for _, it := range items {
			if cm, ok := it.(map[string]interface{}); ok {
				if c := buildContactInfo(cm); c != nil {
					out = append(out, *c)
				}
			}
		}
	}

	return out
}

func buildContactInfo(cm map[string]interface{}) *contactInfo {
	name := strings.TrimSpace(firstString(cm, "displayName", "DisplayName"))
	vcard := firstString(cm, "vcard", "Vcard")
	if name == "" && vcard == "" {
		return nil
	}
	phone := parsePhoneFromVcard(vcard)
	if name == "" {
		name = phone
	}
	return &contactInfo{
		DisplayName: name,
		Phone:       phone,
		Vcard:       vcard,
	}
}

func parsePhoneFromVcard(vcard string) string {
	if vcard == "" {
		return ""
	}
	if m := vcardWaidRegex.FindStringSubmatch(vcard); len(m) > 1 {
		return "+" + m[1]
	}
	if m := vcardTelRegex.FindStringSubmatch(vcard); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func formatContactsText(cs []contactInfo) string {
	if len(cs) == 0 {
		return ""
	}
	if len(cs) == 1 {
		c := cs[0]
		if c.Phone != "" {
			return "📇 Contato: " + c.DisplayName + "\n" + c.Phone
		}
		return "📇 Contato: " + c.DisplayName
	}
	var b strings.Builder
	fmt.Fprintf(&b, "📇 %d contatos compartilhados:", len(cs))
	for _, c := range cs {
		if c.Phone != "" {
			fmt.Fprintf(&b, "\n• %s — %s", c.DisplayName, c.Phone)
		} else {
			fmt.Fprintf(&b, "\n• %s", c.DisplayName)
		}
	}
	return b.String()
}

func vcardsAttachment(cs []contactInfo) *Attachment {
	var b strings.Builder
	for _, c := range cs {
		v := strings.TrimSpace(c.Vcard)
		if v == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\r\n")
		}
		b.WriteString(v)
	}
	if b.Len() == 0 {
		return nil
	}

	name := "contato.vcf"
	if len(cs) > 1 {
		name = "contatos.vcf"
	} else if len(cs) == 1 && cs[0].DisplayName != "" {
		if sanitized := sanitizeContactFilename(cs[0].DisplayName); sanitized != "" {
			name = sanitized + ".vcf"
		}
	}

	return &Attachment{
		Data:     []byte(b.String()),
		Filename: name,
		Mimetype: "text/vcard",
	}
}

func sanitizeContactFilename(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-' || r == '_' || r == ' ':
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func messageMapOf(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		return nil
	}
	if m, ok := data["Message"].(map[string]interface{}); ok && m != nil {
		return m
	}
	if m, ok := data["message"].(map[string]interface{}); ok && m != nil {
		return m
	}
	return nil
}

func extensionFromMimetype(mimetype, kind string) string {
	mt := strings.TrimSpace(mimetype)
	if i := strings.Index(mt, ";"); i >= 0 {
		mt = strings.TrimSpace(mt[:i])
	}
	switch mt {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "audio/ogg":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	case "audio/mp4":
		return ".m4a"
	case "audio/wav":
		return ".wav"
	case "video/mp4":
		return ".mp4"
	case "video/3gpp":
		return ".3gp"
	case "video/webm":
		return ".webm"
	case "application/pdf":
		return ".pdf"
	case "application/msword":
		return ".doc"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return ".docx"
	case "application/vnd.ms-excel":
		return ".xls"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return ".xlsx"
	}
	if i := strings.LastIndex(mt, "/"); i >= 0 && i+1 < len(mt) {
		ext := mt[i+1:]
		if ext != "" && len(ext) <= 6 {
			return "." + ext
		}
	}
	switch kind {
	case "image":
		return ".jpg"
	case "audio":
		return ".ogg"
	case "video":
		return ".mp4"
	case "sticker":
		return ".webp"
	case "document":
		return ".bin"
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

func escapeMultipartQuotes(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}
