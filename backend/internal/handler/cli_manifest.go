package handler

import (
	"yingce/backend/internal/service"

	"github.com/gin-gonic/gin"
)

type cliManifest struct {
	ContractVersion string   `json:"contractVersion"`
	MinCliVersion   string   `json:"minCliVersion"`
	Deprecated      []string `json:"deprecated"`
}

// RegisterCLIRoutes 暴露 CLI 兼容性清单。接口不要求登录，doctor 在授权前也能检查；
// Bearer 访问仍按 read scope 校验。
func RegisterCLIRoutes(api *gin.RouterGroup) {
	api.GET("/cli/manifest", func(c *gin.Context) {
		ok(c, cliManifest{
			ContractVersion: service.CLIContractVersion,
			MinCliVersion:   service.CLIMinCLIVersion,
			Deprecated:      service.CLIDeprecatedCommands,
		})
	})
}
