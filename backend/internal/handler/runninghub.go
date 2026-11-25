package handler

import (
	"net/http"

	"yingce/backend/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterRunningHubRoutes 是独立工作流 Provider 的管理代理，不复用 ModelChannel。
func RegisterRunningHubRoutes(r *gin.RouterGroup, svc *service.Service) {
	fetch := func(c *gin.Context, app bool) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		if err := svc.RequireWorkflowPluginForUser(user.ID, "runninghub-workflow-image"); err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
		var req service.RunningHubWorkflowFetchRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		var result map[string]any
		if app {
			result, err = svc.FetchRunningHubAppInfo(c.Request.Context(), req)
		} else {
			result, err = svc.FetchRunningHubWorkflowInfo(c.Request.Context(), req)
		}
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, result)
	}
	r.POST("/runninghub/workflow-info", func(c *gin.Context) { fetch(c, false) })
	r.POST("/runninghub/app-info", func(c *gin.Context) { fetch(c, true) })

	// 自建 ComfyUI 只解析用户在设置页提交的 API 格式工作流，不访问上游。
	r.POST("/comfyui/workflow-info", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		if err := svc.RequireWorkflowPluginForUser(user.ID, "comfyui-workflow-image"); err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<20)
		var req service.ComfyUIWorkflowImportRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		result, err := svc.ImportComfyUIWorkflow(c.Request.Context(), req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, result)
	})
}
