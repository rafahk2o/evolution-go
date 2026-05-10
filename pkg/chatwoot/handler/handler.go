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

	"github.com/EvolutionAPI/evolution-go/pkg/chatwoot"
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
	chatwootClient     *chatwoot.Client
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
	chatwootClient *chatwoot.Client,
) *Handler {
	return &Handler{
		instanceRepository: instanceRepository,
		sendService:        sendService,
		messageService:     messageService,
		chatwootClient:     chatwootClient,
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

	// Registra o mapeamento Chatwoot msgID → conversation para
	// poder atualizar o status (delivered/read) quando o WhatsApp emitir recibo.
	if h.chatwootClient != nil {
		chatwootMsgID := stringValue(payload.ID)
		conversationID := convertibleString(payload.Conversation["id"])
		h.chatwootClient.RegisterOutgoing(chatwootMsgID, conversationID)
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
			msg, err := h.sendChatwootMedia(payload, attachment, mediaURL, instance, number, messageID)
			if err != nil {
				return nil, err
			}
			return msg, nil
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

func (h *Handler) sendChatwootMedia(payload webhookPayload, attachment map[string]interface{}, mediaURL string, instance *instance_model.Instance, number, messageID string) (interface{}, error) {
	if h.chatwootClient == nil {
		return nil, fmt.Errorf("chatwoot client not configured")
	}
	logger := h.chatwootClient.Logger(instance.Id)

	logger.LogInfo("[%s] chatwoot->wa: download %s", instance.Id, mediaURL)
	fileData, downloadedCT, err := h.chatwootClient.DownloadChatwootMedia(mediaURL, instance.ChatwootAccountToken)
	if err != nil || len(fileData) == 0 {
		logger.LogWarn("[%s] chatwoot->wa: download failed (%v)", instance.Id, err)
		if err == nil {
			err = fmt.Errorf("empty response body")
		}
		return nil, fmt.Errorf("failed to download chatwoot media: %w", err)
	}
	detectedCT := strings.ToLower(strings.TrimSpace(http.DetectContentType(fileData)))
	logger.LogInfo("[%s] chatwoot->wa: download ok, %d bytes ct=%s detected=%s", instance.Id, len(fileData), downloadedCT, detectedCT)

	if invalidAttachmentDownload(downloadedCT, detectedCT, fileData) {
		return nil, fmt.Errorf("chatwoot media download returned non-media content: header=%s detected=%s size=%d", downloadedCT, detectedCT, len(fileData))
	}

	ct := strings.ToLower(strings.TrimSpace(stringValue(attachment["content_type"])))
	if ct == "" || ct == "application/octet-stream" {
		ct = strings.ToLower(strings.TrimSpace(downloadedCT))
	}
	if ct == "" || ct == "application/octet-stream" {
		ct = detectedCT
	}
	filename := strings.TrimSpace(stringValue(attachment["filename"]))
	if filename == "" {
		filename = "file" + extensionFor(ct)
	} else if !strings.Contains(filename, ".") {
		filename = filename + extensionFor(ct)
	}
	if ct == "" || ct == "application/octet-stream" {
		if filenameCT := contentTypeFromFilename(filename); filenameCT != "" {
			ct = filenameCT
		}
	}
	primaryType := pickWhatsAppMediaType(ct, stringValue(attachment["file_type"]))

	build := func(mediaType string) *send_service.MediaStruct {
		return &send_service.MediaStruct{
			Number:   number,
			Type:     mediaType,
			Caption:  payload.Content,
			Id:       messageID,
			Filename: filename,
		}
	}

	logger.LogInfo("[%s] chatwoot->wa: SendMediaFile type=%s filename=%s", instance.Id, primaryType, filename)
	msg, err := h.sendService.SendMediaFile(build(primaryType), fileData, instance)
	if err == nil {
		return msg, nil
	}
	logger.LogWarn("[%s] chatwoot->wa: type %s failed (%v), trying as document", instance.Id, primaryType, err)

	if primaryType != "document" {
		if msg2, err2 := h.sendService.SendMediaFile(build("document"), fileData, instance); err2 == nil {
			logger.LogInfo("[%s] chatwoot->wa: sent as document", instance.Id)
			return msg2, nil
		} else {
			logger.LogError("[%s] chatwoot->wa: document also failed: %v", instance.Id, err2)
			err = fmt.Errorf("primary=%v document=%v", err, err2)
		}
	}

	return nil, err
}

func invalidAttachmentDownload(headerCT, detectedCT string, data []byte) bool {
	headerCT = strings.ToLower(strings.TrimSpace(headerCT))
	detectedCT = strings.ToLower(strings.TrimSpace(detectedCT))
	for _, ct := range []string{headerCT, detectedCT} {
		if ct == "" {
			continue
		}
		if strings.HasPrefix(ct, "text/") ||
			strings.Contains(ct, "html") ||
			strings.Contains(ct, "json") ||
			strings.Contains(ct, "xml") {
			return true
		}
	}

	limit := len(data)
	if limit > 256 {
		limit = 256
	}
	prefix := strings.ToLower(strings.TrimSpace(string(data[:limit])))
	return strings.HasPrefix(prefix, "<!doctype") ||
		strings.HasPrefix(prefix, "<html") ||
		strings.HasPrefix(prefix, "{") ||
		strings.HasPrefix(prefix, "[")
}

func extensionFor(ct string) string {
	switch ct {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/heic", "image/heif":
		return ".heic"
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	case "video/webm":
		return ".webm"
	case "video/3gpp":
		return ".3gp"
	case "audio/ogg":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	case "audio/mp4", "audio/m4a", "audio/x-m4a":
		return ".m4a"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/webm":
		return ".webm"
	case "audio/aac":
		return ".aac"
	case "application/pdf":
		return ".pdf"
	}
	if i := strings.LastIndex(ct, "/"); i >= 0 && i+1 < len(ct) {
		ext := ct[i+1:]
		if len(ext) <= 6 {
			return "." + ext
		}
	}
	return ""
}

func contentTypeFromFilename(filename string) string {
	lower := strings.ToLower(strings.TrimSpace(filename))
	switch {
	case strings.HasSuffix(lower, ".mp4"):
		return "video/mp4"
	case strings.HasSuffix(lower, ".mov"):
		return "video/quicktime"
	case strings.HasSuffix(lower, ".webm"):
		return "video/webm"
	case strings.HasSuffix(lower, ".3gp"):
		return "video/3gpp"
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lower, ".ogg"):
		return "audio/ogg"
	case strings.HasSuffix(lower, ".mp3"):
		return "audio/mpeg"
	case strings.HasSuffix(lower, ".m4a"):
		return "audio/mp4"
	case strings.HasSuffix(lower, ".wav"):
		return "audio/wav"
	}
	return ""
}

func pickWhatsAppMediaType(contentType, chatwootType string) string {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	switch ct {
	case "image/jpeg", "image/jpg", "image/png", "image/webp":
		return "image"
	case "video/mp4":
		return "video"
	case "audio/ogg", "audio/mpeg", "audio/mp4", "audio/m4a", "audio/wav",
		"audio/webm", "audio/aac", "audio/x-wav":
		return "audio"
	}
	if strings.HasPrefix(ct, "image/") {
		return "document"
	}
	if strings.HasPrefix(ct, "video/") {
		return "document"
	}
	if strings.HasPrefix(ct, "audio/") {
		return "audio"
	}
	switch strings.ToLower(strings.TrimSpace(chatwootType)) {
	case "image":
		return "image"
	case "video":
		return "video"
	case "audio":
		return "audio"
	}
	return "document"
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
