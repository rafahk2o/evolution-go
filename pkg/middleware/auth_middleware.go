package auth_middleware

import (
	"net/http"

	company_service "github.com/EvolutionAPI/evolution-go/pkg/company/service"
	"github.com/EvolutionAPI/evolution-go/pkg/config"
	instance_service "github.com/EvolutionAPI/evolution-go/pkg/instance/service"
	"github.com/gin-gonic/gin"
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
	token := ctx.GetHeader("apikey")
	if token == "" {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	company, err := m.companyService.AuthenticateAPIKey(token)
	if err != nil {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	ctx.Set("company", company)
	ctx.Set("companyId", company.Id)

	ctx.Next()
}

func (m middleware) AuthMaster(ctx *gin.Context) {
	token := ctx.GetHeader("apikey")
	if token == "" {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	if token != m.config.GlobalApiKey {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	ctx.Next()
}

func NewMiddleware(config *config.Config, companyService company_service.CompanyService, instanceService instance_service.InstanceService) *middleware {
	return &middleware{config: config, companyService: companyService, instanceService: instanceService}
}
