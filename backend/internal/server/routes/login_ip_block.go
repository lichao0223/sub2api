package routes

import (
	"net"

	appmiddleware "github.com/Wei-Shaw/sub2api/internal/middleware"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

func registerLoginIPBlockRoutes(group *gin.RouterGroup, blocker *appmiddleware.LoginIPBlocker) {
	group.GET("/login-ip-blocks", func(c *gin.Context) {
		current, err := blocker.ListCurrent(c.Request.Context())
		if err != nil {
			response.InternalError(c, "Failed to load blocked IPs")
			return
		}
		history, err := blocker.ListHistory(c.Request.Context())
		if err != nil {
			response.InternalError(c, "Failed to load blocked IP history")
			return
		}
		response.Success(c, gin.H{"current": current, "history": history})
	})

	group.DELETE("/login-ip-blocks/:ip", func(c *gin.Context) {
		clientIP := c.Param("ip")
		if net.ParseIP(clientIP) == nil {
			response.BadRequest(c, "Invalid IP address")
			return
		}
		if err := blocker.Unblock(c.Request.Context(), clientIP); err != nil {
			response.InternalError(c, "Failed to unblock IP")
			return
		}
		response.Success(c, gin.H{"message": "IP unblocked"})
	})
}
