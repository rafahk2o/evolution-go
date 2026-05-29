package company_repository

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	company_model "github.com/EvolutionAPI/evolution-go/pkg/company/model"
	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	"gorm.io/gorm"
)

const DefaultCompanyName = "default"

type CompanyRepository interface {
	Create(name string, apiKey string) (*company_model.Company, error)
	GetAll() ([]*company_model.Company, error)
	GetByAPIKey(apiKey string) (*company_model.Company, error)
	GetByID(id string) (*company_model.Company, error)
	CountInstances(companyID string) (int64, error)
	Delete(id string) error
	EnsureDefaultCompany(apiKey string) (*company_model.Company, error)
	BackfillInstances(companyID string) error
}

type companyRepository struct {
	db *gorm.DB
}

func HashAPIKey(apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:])
}

func (r *companyRepository) Create(name string, apiKey string) (*company_model.Company, error) {
	company := company_model.Company{
		Name:       strings.TrimSpace(name),
		ApiKeyHash: HashAPIKey(apiKey),
		Active:     true,
	}

	if err := r.db.Create(&company).Error; err != nil {
		return nil, err
	}

	return &company, nil
}

func (r *companyRepository) GetAll() ([]*company_model.Company, error) {
	var companies []*company_model.Company
	if err := r.db.Order("created_at asc").Find(&companies).Error; err != nil {
		return nil, err
	}
	return companies, nil
}

func (r *companyRepository) GetByAPIKey(apiKey string) (*company_model.Company, error) {
	var company company_model.Company
	err := r.db.Where("api_key_hash = ? AND active = ?", HashAPIKey(apiKey), true).First(&company).Error
	if err != nil {
		return nil, err
	}
	return &company, nil
}

func (r *companyRepository) GetByID(id string) (*company_model.Company, error) {
	var company company_model.Company
	if err := r.db.Where("id = ?", id).First(&company).Error; err != nil {
		return nil, err
	}
	return &company, nil
}

func (r *companyRepository) CountInstances(companyID string) (int64, error) {
	var count int64
	if err := r.db.Model(&instance_model.Instance{}).Where("company_id = ?", companyID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *companyRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&company_model.Company{}).Error
}

func (r *companyRepository) EnsureDefaultCompany(apiKey string) (*company_model.Company, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("default company api key is required")
	}

	if company, err := r.GetByAPIKey(apiKey); err == nil {
		return company, nil
	}

	var company company_model.Company
	err := r.db.Where("name = ?", DefaultCompanyName).First(&company).Error
	if err == nil {
		company.ApiKeyHash = HashAPIKey(apiKey)
		company.Active = true
		if err := r.db.Save(&company).Error; err != nil {
			return nil, err
		}
		return &company, nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}

	return r.Create(DefaultCompanyName, apiKey)
}

func (r *companyRepository) BackfillInstances(companyID string) error {
	return r.db.Model(&instance_model.Instance{}).
		Where("company_id IS NULL OR company_id = ?", "").
		Update("company_id", companyID).Error
}

func NewCompanyRepository(db *gorm.DB) CompanyRepository {
	return &companyRepository{db: db}
}
