package chatwoot_handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	instance_repository "github.com/EvolutionAPI/evolution-go/pkg/instance/repository"
	message_service "github.com/EvolutionAPI/evolution-go/pkg/message/service"
	send_service "github.com/EvolutionAPI/evolution-go/pkg/sendMessage/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	instanceRepository instance_repository.InstanceRepository
	sendService        send_service.SendService
	messageService     message_service.MessageService
	httpClient         *http.Client
}

type webhookPayload struct {
	Event        string                   `json:"event"`
	ID           interface{}              `json:"id"`
	Content      string                   `json:"content"`
	MessageType  interface{}              `json:"message_type"`
	Private      bool                     `json:"private"`
	Conversation map[string]interface{}   `json:"conversation"`
	Attachments  []map[string]interface{} `json:"attachments"`
}

func NewHandler(
	instanceRepository instance_repository.InstanceRepository,
	sendService send_service.SendService,
	messageService message_service.MessageService,
) *Handler {
	return &Handler{
		instanceRepository: instanceRepository,
		sendService:        sendService,
		messageService:     messageService,
		httpClient:         &http.Client{Timeout: 15 * time.Second},
	}
}

func (h *Handler) Webhook(ctx *gin.Context) {
	instance, err := h.findInstance(ctx.Param("instance"))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	if !h.validToken(ctx.Param("token"), instance) {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	var payload webhookPayload
	if err := ctx.ShouldBindJSON(&payload); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !shouldSendToWhatsApp(payload) {
		ctx.JSON(http.StatusOK, gin.H{"message": "ignored"})
		return
	}

	number := extractChatwootPhone(payload)
	if number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "contact phone not found in chatwoot payload"})
		return
	}

	messageID := stringValue(payload.ID)
	if messageID != "" {
		messageID = "chatwoot_" + messageID
	}
	message, err := h.send(payload, instance, number, messageID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Quando o agente responde no Chatwoot, marcar as mensagens incoming
	// anteriores como lidas no WhatsApp (proxy: agent reply ⇒ tudo lido).
	go h.markIncomingAsRead(payload, instance, number)

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// markIncomingAsRead busca as mensagens da conversa no Chatwoot e marca a última
// mensagem incoming (do cliente) como lida no WhatsApp. A whatsmeow propaga o
// recibo de leitura para todas as mensagens anteriores da mesma conversa.
func (h *Handler) markIncomingAsRead(payload webhookPayload, instance *instance_model.Instance, phone string) {
	if h.messageService == nil {
		return
	}
	if instance.ChatwootURL == "" || instance.ChatwootAccountToken == "" || instance.ChatwootAccountID == "" {
		return
	}

	conversationID := convertibleString(payload.Conversation["id"])
	if conversationID == "" {
		return
	}

	url := fmt.Sprintf("%s/api/v1/accounts/%s/conversations/%s/messages",
		strings.TrimRight(instance.ChatwootURL, "/"),
		instance.ChatwootAccountID,
		conversationID,
	)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return
	}
	req.Header.Set("api_access_token", instance.ChatwootAccountToken)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	var parsed struct {
		Payload []struct {
			MessageType int     `json:"message_type"`
			SourceID    string  `json:"source_id"`
			CreatedAt   float64 `json:"created_at"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return
	}

	var ids []string
	for _, m := range parsed.Payload {
		if m.MessageType != 0 {
			continue
		}
		if m.SourceID == "" || strings.HasPrefix(m.SourceID, "chatwoot_") {
			continue
		}
		ids = append(ids, m.SourceID)
	}
	if len(ids) == 0 {
		return
	}

	_, _ = h.messageService.MarkRead(&message_service.MarkReadStruct{
		Id:     ids,
		Number: phone,
	}, instance)
}

func convertibleString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		if v == 0 {
			return ""
		}
		return strconv.FormatInt(int64(v), 10)
	case int:
		if v == 0 {
			return ""
		}
		return strconv.Itoa(v)
	case int64:
		if v == 0 {
			return ""
		}
		return strconv.FormatInt(v, 10)
	default:
		return ""
	}
}

func (h *Handler) findInstance(identifier string) (*instance_model.Instance, error) {
	if identifier == "" {
		return nil, http.ErrMissingFile
	}

	if instance, err := h.instanceRepository.GetInstanceByName(identifier); err == nil {
		return instance, nil
	}
	if _, err := uuid.Parse(identifier); err == nil {
		if instance, err := h.instanceRepository.GetInstanceByID(identifier); err == nil {
			return instance, nil
		}
	}
	return h.instanceRepository.GetInstanceByToken(identifier)
}

func (h *Handler) validToken(token string, instance *instance_model.Instance) bool {
	if token == "" {
		return false
	}
	if instance.ChatwootWebhookToken != "" {
		return token == instance.ChatwootWebhookToken
	}
	return token == instance.Token || token == instance.Name || token == instance.Id
}

func shouldSendToWhatsApp(payload webhookPayload) bool {
	if payload.Event != "message_created" {
		return false
	}
	if payload.Private {
		return false
	}
	if strings.TrimSpace(payload.Content) == "" && len(payload.Attachments) == 0 {
		return false
	}

	messageType := strings.ToLower(stringValue(payload.MessageType))
	return messageType == "outgoing" || messageType == "template" || messageType == "1" || messageType == "3"
}

func (h *Handler) send(payload webhookPayload, instance *instance_model.Instance, number string, messageID string) (interface{}, error) {
	attachment := firstSupportedAttachment(payload.Attachments)
	if attachment != nil {
		mediaURL := h.absoluteChatwootURL(instance, stringValue(attachment["data_url"]))
		if mediaURL != "" {
			mediaType := chatwootAttachmentType(stringValue(attachment["file_type"]))
			return h.sendService.SendMediaUrl(&send_service.MediaStruct{
				Number:  number,
				Url:     mediaURL,
				Type:    mediaType,
				Caption: payload.Content,
				Id:      messageID,
			}, instance)
		}
	}

	content := payload.Content
	if strings.TrimSpace(content) == "" {
		content = "[attachment]"
	}

	return h.sendService.SendText(&send_service.TextStruct{
		Number: number,
		Text:   content,
		Id:     messageID,
	}, instance)
}

func firstSupportedAttachment(attachments []map[string]interface{}) map[string]interface{} {
	for _, attachment := range attachments {
		if stringValue(attachment["data_url"]) == "" {
			continue
		}
		switch chatwootAttachmentType(stringValue(attachment["file_type"])) {
		case "image", "video", "audio", "document":
			return attachment
		}
	}
	return nil
}

func chatwootAttachmentType(fileType string) string {
	switch strings.ToLower(fileType) {
	case "image":
		return "image"
	case "video":
		return "video"
	case "audio":
		return "audio"
	default:
		return "document"
	}
}

func (h *Handler) absoluteChatwootURL(instance *instance_model.Instance, value string) string {
	if value == "" || strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	return strings.TrimRight(instance.ChatwootURL, "/") + "/" + strings.TrimLeft(value, "/")
}

func extractChatwootPhone(payload webhookPayload) string {
	for _, path := range [][]string{
		{"meta", "sender", "phone_number"},
		{"contact", "phone_number"},
		{"contact_inbox", "source_id"},
	} {
		if value := nestedString(payload.Conversation, path...); value != "" {
			if phone := phoneFromJID(value); phone != "" {
				return phone
			}
		}
	}
	return ""
}

func nestedString(data map[string]interface{}, path ...string) string {
	var current interface{} = data
	for _, key := range path {
		currentMap, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current = currentMap[key]
	}
	return stringValue(current)
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
