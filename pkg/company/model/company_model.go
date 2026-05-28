package company_model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Company struct {
	Id         string    `json:"id" gorm:"type:uuid;primaryKey"`
	Name       string    `json:"name" gorm:"uniqueIndex;not null"`
	ApiKeyHash string    `json:"-" gorm:"column:api_key_hash;uniqueIndex;not null"`
	Active     bool      `json:"active" gorm:"default:true"`
	CreatedAt  time.Time `json:"createdAt" gorm:"autoCreateTime"`
}

func (m *Company) BeforeCreate(tx *gorm.DB) (err error) {
	if m.Id == "" {
		m.Id = uuid.New().String()
	}
	return
}
