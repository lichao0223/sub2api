package routes

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAdminUserAPIKeyRouteDoesNotConflictWithUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	users := router.Group("/api/v1/admin/users")
	users.GET("/:id", func(c *gin.Context) { c.Status(http.StatusOK) })
	users.POST("/:id/api-keys", func(c *gin.Context) { c.Status(http.StatusCreated) })
}
