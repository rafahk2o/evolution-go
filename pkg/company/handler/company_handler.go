package company_handler

import (
	"net/http"
	"strings"

	company_service "github.com/EvolutionAPI/evolution-go/pkg/company/service"
	instance_service "github.com/EvolutionAPI/evolution-go/pkg/instance/service"
	"github.com/gin-gonic/gin"
)

type CompanyHandler interface {
	Create(ctx *gin.Context)
	All(ctx *gin.Context)
	Instances(ctx *gin.Context)
}

type companyHandler struct {
	companyService  company_service.CompanyService
	instanceService instance_service.InstanceService
}

func (h *companyHandler) Create(ctx *gin.Context) {
	var data company_service.CreateStruct
	if err := ctx.ShouldBindJSON(&data); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.companyService.Create(&data)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": result})
}

func (h *companyHandler) All(ctx *gin.Context) {
	companies, err := h.companyService.GetAll()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": companies})
}

func (h *companyHandler) Instances(ctx *gin.Context) {
	companyID := strings.TrimSpace(ctx.Param("companyId"))
	if companyID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "companyId is required"})
		return
	}

	instances, err := h.instanceService.GetAll(companyID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": instances})
}

func NewCompanyHandler(companyService company_service.CompanyService, instanceService instance_service.InstanceService) CompanyHandler {
	return &companyHandler{companyService: companyService, instanceService: instanceService}
}
