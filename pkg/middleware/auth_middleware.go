package auth_middleware

import (
	"errors"
	"log"
	"net/http"
	"strings"

	company_service "github.com/EvolutionAPI/evolution-go/pkg/company/service"
	"github.com/EvolutionAPI/evolution-go/pkg/config"
	instance_service "github.com/EvolutionAPI/evolution-go/pkg/instance/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Middleware interface {
	Auth(ctx *gin.Context)
	AuthAdmin(ctx *gin.Context)
	AuthMaster(ctx *gin.Context)
}

type middleware struct {
	config          *config.Config
	companyService  company_service.CompanyService
	instanceService instance_service.InstanceService
}

func (m middleware) Auth(ctx *gin.Context) {
	token := ctx.GetHeader("apikey")
	if token == "" {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	instance, err := m.instanceService.GetInstanceByToken(token)
	if err != nil {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	ctx.Set("instance", instance)
	ctx.Set("companyId", instance.CompanyID)

	ctx.Next()
}

func (m middleware) AuthAdmin(ctx *gin.Context) {
	token := strings.TrimSpace(ctx.GetHeader("apikey"))
	if token == "" {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	if token == strings.TrimSpace(m.config.GlobalApiKey) {
		if companyID := strings.TrimSpace(ctx.GetHeader("X-Company-Id")); companyID != "" {
			ctx.Set("companyId", companyID)
			ctx.Set("isMaster", true)
			ctx.Next()
			return
		}

		company, err := m.companyService.GetDefaultCompany()
		if err != nil {
			log.Printf("default company authentication lookup failed: %v", err)
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}

		ctx.Set("company", company)
		ctx.Set("companyId", company.Id)
		ctx.Set("isMaster", true)
		ctx.Next()
		return
	}

	company, err := m.companyService.AuthenticateAPIKey(token)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
			return
		}
		log.Printf("company API key authentication lookup failed: %v", err)
		ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	ctx.Set("company", company)
	ctx.Set("companyId", company.Id)

	ctx.Next()
}

func (m middleware) AuthMaster(ctx *gin.Context) {
	token := strings.TrimSpace(ctx.GetHeader("apikey"))
	if token == "" {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	if token != strings.TrimSpace(m.config.GlobalApiKey) {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	ctx.Next()
}

func NewMiddleware(config *config.Config, companyService company_service.CompanyService, instanceService instance_service.InstanceService) *middleware {
	return &middleware{config: config, companyService: companyService, instanceService: instanceService}
}
