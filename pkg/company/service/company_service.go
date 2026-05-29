package company_service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	company_model "github.com/EvolutionAPI/evolution-go/pkg/company/model"
	company_repository "github.com/EvolutionAPI/evolution-go/pkg/company/repository"
)

type CompanyService interface {
	Create(data *CreateStruct) (*CreateResult, error)
	GetAll() ([]*company_model.Company, error)
	AuthenticateAPIKey(apiKey string) (*company_model.Company, error)
	Delete(id string) error
	EnsureDefaultCompany(apiKey string) (*company_model.Company, error)
	BackfillInstances(companyID string) error
}

type companies struct {
	companyRepository company_repository.CompanyRepository
}

type CreateStruct struct {
	Name   string `json:"name"`
	ApiKey string `json:"apiKey"`
}

type CreateResult struct {
	Company *company_model.Company `json:"company"`
	ApiKey  string                 `json:"apiKey,omitempty"`
}

func (c *companies) Create(data *CreateStruct) (*CreateResult, error) {
	if data == nil {
		return nil, fmt.Errorf("company data is required")
	}

	name := strings.TrimSpace(data.Name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	apiKey := strings.TrimSpace(data.ApiKey)
	if apiKey == "" {
		var err error
		apiKey, err = generateAPIKey()
		if err != nil {
			return nil, err
		}
	}

	company, err := c.companyRepository.Create(name, apiKey)
	if err != nil {
		return nil, err
	}

	return &CreateResult{Company: company, ApiKey: apiKey}, nil
}

func (c *companies) GetAll() ([]*company_model.Company, error) {
	return c.companyRepository.GetAll()
}

func (c *companies) AuthenticateAPIKey(apiKey string) (*company_model.Company, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("api key is required")
	}
	return c.companyRepository.GetByAPIKey(apiKey)
}

func (c *companies) Delete(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("company id is required")
	}

	company, err := c.companyRepository.GetByID(id)
	if err != nil {
		return err
	}

	if company.Name == company_repository.DefaultCompanyName {
		return fmt.Errorf("a empresa 'default' nao pode ser excluida")
	}

	count, err := c.companyRepository.CountInstances(id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("a empresa possui %d instancia(s); remova-as antes de excluir", count)
	}

	return c.companyRepository.Delete(id)
}

func (c *companies) EnsureDefaultCompany(apiKey string) (*company_model.Company, error) {
	return c.companyRepository.EnsureDefaultCompany(apiKey)
}

func (c *companies) BackfillInstances(companyID string) error {
	return c.companyRepository.BackfillInstances(companyID)
}

func generateAPIKey() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate api key: %w", err)
	}
	return "evo_co_" + hex.EncodeToString(buf), nil
}

func NewCompanyService(companyRepository company_repository.CompanyRepository) CompanyService {
	return &companies{companyRepository: companyRepository}
}
