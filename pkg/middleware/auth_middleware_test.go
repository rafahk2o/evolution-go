package auth_middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	company_model "github.com/EvolutionAPI/evolution-go/pkg/company/model"
	company_service "github.com/EvolutionAPI/evolution-go/pkg/company/service"
	"github.com/EvolutionAPI/evolution-go/pkg/config"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type authCompanyService struct {
	ensuredKey       string
	authenticatedKey string
	authenticated    *company_model.Company
	authenticateErr  error
	defaultCompany   *company_model.Company
	defaultErr       error
}

func (s *authCompanyService) Create(*company_service.CreateStruct) (*company_service.CreateResult, error) {
	return nil, nil
}

func (s *authCompanyService) GetAll() ([]*company_model.Company, error) { return nil, nil }

func (s *authCompanyService) AuthenticateAPIKey(apiKey string) (*company_model.Company, error) {
	s.authenticatedKey = apiKey
	if s.authenticateErr != nil {
		return nil, s.authenticateErr
	}
	if s.authenticated != nil {
		return s.authenticated, nil
	}
	return &company_model.Company{Id: "tenant-company"}, nil
}

func (s *authCompanyService) GetDefaultCompany() (*company_model.Company, error) {
	if s.defaultErr != nil {
		return nil, s.defaultErr
	}
	if s.defaultCompany != nil {
		return s.defaultCompany, nil
	}
	return &company_model.Company{Id: "default-company"}, nil
}

func (s *authCompanyService) Delete(string) error { return nil }

func (s *authCompanyService) EnsureDefaultCompany(apiKey string) (*company_model.Company, error) {
	s.ensuredKey = apiKey
	return &company_model.Company{Id: "default-company"}, nil
}

func (s *authCompanyService) BackfillInstances(string) error { return nil }

func TestAuthAdminNormalizesGlobalAPIKeyAndUsesDefaultCompany(t *testing.T) {
	gin.SetMode(gin.TestMode)
	companies := &authCompanyService{}
	middleware := middleware{
		config:         &config.Config{GlobalApiKey: "global-secret"},
		companyService: companies,
	}

	router := gin.New()
	router.GET("/instance/all", middleware.AuthAdmin, func(ctx *gin.Context) {
		companyID, _ := ctx.Get("companyId")
		isMaster, _ := ctx.Get("isMaster")
		if companyID != "default-company" || isMaster != true {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		ctx.Status(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodGet, "/instance/all", nil)
	request.Header.Set("apikey", "  global-secret  ")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	if companies.ensuredKey != "" {
		t.Fatal("request authentication synchronized the default company")
	}
	if companies.authenticatedKey != "" {
		t.Fatalf("global key used tenant authentication: %q", companies.authenticatedKey)
	}
}

func TestAuthAdminNormalizesCompanyAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	companies := &authCompanyService{
		authenticated: &company_model.Company{Id: "tenant-company"},
	}
	middleware := middleware{
		config:         &config.Config{GlobalApiKey: "global-secret"},
		companyService: companies,
	}

	router := gin.New()
	router.GET("/instance/all", middleware.AuthAdmin, func(ctx *gin.Context) {
		companyID, _ := ctx.Get("companyId")
		if companyID != "tenant-company" {
			ctx.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		ctx.Status(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodGet, "/instance/all", nil)
	request.Header.Set("apikey", "  evo_co_company-secret  ")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	if companies.authenticatedKey != "evo_co_company-secret" {
		t.Fatalf("company key was not normalized: %q", companies.authenticatedKey)
	}
}

func TestAuthAdminClassifiesAuthenticationErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		key        string
		companies  *authCompanyService
		wantStatus int
	}{
		{"unknown company key", "unknown", &authCompanyService{authenticateErr: gorm.ErrRecordNotFound}, http.StatusUnauthorized},
		{"company database failure", "company-key", &authCompanyService{authenticateErr: errors.New("database unavailable")}, http.StatusInternalServerError},
		{"default database failure", "global-secret", &authCompanyService{defaultErr: errors.New("database unavailable")}, http.StatusInternalServerError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			middleware := middleware{
				config:         &config.Config{GlobalApiKey: "global-secret"},
				companyService: test.companies,
			}
			router := gin.New()
			router.GET("/instance/all", middleware.AuthAdmin)
			request := httptest.NewRequest(http.MethodGet, "/instance/all", nil)
			request.Header.Set("apikey", test.key)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("expected %d, got %d", test.wantStatus, response.Code)
			}
		})
	}
}

func TestAuthMasterNormalizesGlobalAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	middleware := middleware{config: &config.Config{GlobalApiKey: "global-secret"}}
	router := gin.New()
	router.GET("/company/all", middleware.AuthMaster, func(ctx *gin.Context) {
		ctx.Status(http.StatusOK)
	})
	request := httptest.NewRequest(http.MethodGet, "/company/all", nil)
	request.Header.Set("apikey", "  global-secret  ")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
}
