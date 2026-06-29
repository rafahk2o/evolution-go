package auth_middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	company_model "github.com/EvolutionAPI/evolution-go/pkg/company/model"
	company_service "github.com/EvolutionAPI/evolution-go/pkg/company/service"
	"github.com/EvolutionAPI/evolution-go/pkg/config"
	"github.com/gin-gonic/gin"
)

type authCompanyService struct {
	ensuredKey       string
	authenticatedKey string
}

func (s *authCompanyService) Create(*company_service.CreateStruct) (*company_service.CreateResult, error) {
	return nil, nil
}

func (s *authCompanyService) GetAll() ([]*company_model.Company, error) { return nil, nil }

func (s *authCompanyService) AuthenticateAPIKey(apiKey string) (*company_model.Company, error) {
	s.authenticatedKey = apiKey
	return &company_model.Company{Id: "tenant-company"}, nil
}

func (s *authCompanyService) Delete(string) error { return nil }

func (s *authCompanyService) EnsureDefaultCompany(apiKey string) (*company_model.Company, error) {
	s.ensuredKey = apiKey
	return &company_model.Company{Id: "default-company"}, nil
}

func (s *authCompanyService) BackfillInstances(string) error { return nil }

func TestAuthAdminKeepsGlobalAPIKeyAuthoritative(t *testing.T) {
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
	request.Header.Set("apikey", "global-secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	if companies.ensuredKey != "global-secret" {
		t.Fatalf("default company was not synchronized with GLOBAL_API_KEY: %q", companies.ensuredKey)
	}
	if companies.authenticatedKey != "" {
		t.Fatalf("global key used tenant authentication: %q", companies.authenticatedKey)
	}
}
