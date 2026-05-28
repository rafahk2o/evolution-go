package instance_model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Instance struct {
	Id               string    `json:"id" gorm:"type:uuid;primaryKey"`
	CompanyID        string    `json:"companyId" gorm:"column:company_id;type:uuid;index"`
	Name             string    `json:"name"`
	Token            string    `json:"token" gorm:"unique"`
	Webhook          string    `json:"webhook"`
	RabbitmqEnable   string    `json:"rabbitmqEnable"`
	WebSocketEnable  string    `json:"websocketEnable"`
	NatsEnable       string    `json:"natsEnable"`
	Jid              string    `json:"jid" gorm:"column:jid"`
	Qrcode           string    `json:"qrcode" gorm:"type:text"`
	Connected        bool      `json:"connected"`
	Expiration       int64     `json:"expiration"`
	DisconnectReason string    `json:"disconnect_reason"`
	Events           string    `json:"events"`
	OsName           string    `json:"os_name"`
	Proxy            string    `json:"proxy"`
	ClientName       string    `json:"client_name"`
	CreatedAt        time.Time `json:"createdAt" gorm:"autoCreateTime"`

	// Advanced Settings
	AlwaysOnline  bool   `json:"alwaysOnline" gorm:"default:false"`
	RejectCall    bool   `json:"rejectCall" gorm:"default:false"`
	MsgRejectCall string `json:"msgRejectCall" gorm:"default:''"`
	ReadMessages  bool   `json:"readMessages" gorm:"default:false"`
	IgnoreGroups  bool   `json:"ignoreGroups" gorm:"default:false"`
	IgnoreStatus  bool   `json:"ignoreStatus" gorm:"default:false"`

	// Chatwoot API Inbox Settings
	ChatwootEnabled         bool   `json:"chatwootEnabled" gorm:"column:chatwoot_enabled;default:false"`
	ChatwootURL             string `json:"chatwootUrl" gorm:"column:chatwoot_url;default:''"`
	ChatwootAccountID       string `json:"chatwootAccountId" gorm:"column:chatwoot_account_id;default:''"`
	ChatwootAccountToken    string `json:"chatwootAccountToken" gorm:"column:chatwoot_account_token;default:''"`
	ChatwootInboxID         string `json:"chatwootInboxId" gorm:"column:chatwoot_inbox_id;default:''"`
	ChatwootInboxIdentifier string `json:"chatwootInboxIdentifier" gorm:"column:chatwoot_inbox_identifier;default:''"`
	ChatwootWebhookToken    string `json:"chatwootWebhookToken" gorm:"column:chatwoot_webhook_token;default:''"`
	ChatwootHMACToken       string `json:"chatwootHmacToken" gorm:"column:chatwoot_hmac_token;default:''"`
	ChatwootEnableGroups    bool   `json:"chatwootEnableGroups" gorm:"column:chatwoot_enable_groups;default:false"`
}

// AdvancedSettings representa as configurações avançadas de uma instância
type AdvancedSettings struct {
	AlwaysOnline  bool   `json:"alwaysOnline"`
	RejectCall    bool   `json:"rejectCall"`
	MsgRejectCall string `json:"msgRejectCall"`
	ReadMessages  bool   `json:"readMessages"`
	IgnoreGroups  bool   `json:"ignoreGroups"`
	IgnoreStatus  bool   `json:"ignoreStatus"`
}

// ChatwootSettings representa as credenciais do Chatwoot por instância/caixa de entrada.
type ChatwootSettings struct {
	Enabled         bool   `json:"enabled"`
	URL             string `json:"url"`
	AccountID       string `json:"accountId"`
	AccountToken    string `json:"accountToken"`
	InboxID         string `json:"inboxId"`
	InboxIdentifier string `json:"inboxIdentifier"`
	WebhookToken    string `json:"webhookToken"`
	HMACToken       string `json:"hmacToken"`
	EnableGroups    bool   `json:"enableGroups"`
}

func (m *Instance) BeforeCreate(tx *gorm.DB) (err error) {
	if m.Id == "" {
		m.Id = uuid.New().String()
	}
	return
}
