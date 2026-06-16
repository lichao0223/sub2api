package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterIntegrationRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	adminAuth middleware.AdminAuthMiddleware,
) {
	integrations := v1.Group("/integrations")
	integrations.Use(gin.HandlerFunc(adminAuth))
	{
		users := integrations.Group("/users")
		{
			users.POST("", h.Integration.User.Create)
			users.DELETE("/:external_user_id", h.Integration.User.DeleteByExternalID)
			users.POST("/sync", h.Integration.User.Sync)
		}
	}
}
