package admin

import (
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminAPIKeyHandler handles admin API key management
type AdminAPIKeyHandler struct {
	adminService  service.AdminService
	apiKeyService *service.APIKeyService
}

// NewAdminAPIKeyHandler creates a new admin API key handler
func NewAdminAPIKeyHandler(adminService service.AdminService, apiKeyServices ...*service.APIKeyService) *AdminAPIKeyHandler {
	h := &AdminAPIKeyHandler{
		adminService: adminService,
	}
	if len(apiKeyServices) > 0 {
		h.apiKeyService = apiKeyServices[0]
	}
	return h
}

type AdminCreateAPIKeyRequest struct {
	Name          string   `json:"name" binding:"required"`
	GroupID       *int64   `json:"group_id"`
	CustomKey     *string  `json:"custom_key"`
	IPWhitelist   []string `json:"ip_whitelist"`
	IPBlacklist   []string `json:"ip_blacklist"`
	Quota         *float64 `json:"quota"`
	ExpiresInDays *int     `json:"expires_in_days"`
	RateLimit5h   *float64 `json:"rate_limit_5h"`
	RateLimit1d   *float64 `json:"rate_limit_1d"`
	RateLimit7d   *float64 `json:"rate_limit_7d"`
	Concurrency   *int     `json:"concurrency" binding:"omitempty,gte=0"`
}

type AdminUpdateAPIKeyRequest struct {
	Name                string   `json:"name"`
	GroupID             *int64   `json:"group_id"` // nil=不修改, 0=解绑, >0=绑定到目标分组
	Status              string   `json:"status" binding:"omitempty,oneof=active inactive"`
	IPWhitelist         []string `json:"ip_whitelist"`
	IPBlacklist         []string `json:"ip_blacklist"`
	Quota               *float64 `json:"quota"`
	ExpiresAt           *string  `json:"expires_at"`
	ResetQuota          *bool    `json:"reset_quota"`
	RateLimit5h         *float64 `json:"rate_limit_5h"`
	RateLimit1d         *float64 `json:"rate_limit_1d"`
	RateLimit7d         *float64 `json:"rate_limit_7d"`
	ResetRateLimitUsage *bool    `json:"reset_rate_limit_usage"`
	Concurrency         *int     `json:"concurrency" binding:"omitempty,gte=0"`
}

// Create handles creating an API key for a user.
// POST /api/v1/admin/users/:id/api-keys
func (h *AdminAPIKeyHandler) Create(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		id = c.Param("user_id")
	}
	userID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	var req AdminCreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	svcReq := service.CreateAPIKeyRequest{
		Name:          req.Name,
		CustomKey:     req.CustomKey,
		IPWhitelist:   req.IPWhitelist,
		IPBlacklist:   req.IPBlacklist,
		ExpiresInDays: req.ExpiresInDays,
	}
	if req.Quota != nil {
		svcReq.Quota = *req.Quota
	}
	if req.RateLimit5h != nil {
		svcReq.RateLimit5h = *req.RateLimit5h
	}
	if req.RateLimit1d != nil {
		svcReq.RateLimit1d = *req.RateLimit1d
	}
	if req.RateLimit7d != nil {
		svcReq.RateLimit7d = *req.RateLimit7d
	}
	if req.Concurrency != nil {
		svcReq.Concurrency = *req.Concurrency
	}

	key, err := h.apiKeyService.Create(c.Request.Context(), userID, svcReq)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if req.GroupID != nil {
		result, err := h.adminService.AdminUpdateAPIKeyGroupID(c.Request.Context(), key.ID, req.GroupID)
		if err != nil {
			_ = h.apiKeyService.Delete(c.Request.Context(), key.ID, userID)
			response.ErrorFrom(c, err)
			return
		}
		key = result.APIKey
	}

	response.Created(c, dto.APIKeyFromService(key))
}

// UpdateGroup handles updating an API key's admin-managed fields.
// PUT /api/v1/admin/api-keys/:id
func (h *AdminAPIKeyHandler) UpdateGroup(c *gin.Context) {
	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid API key ID")
		return
	}

	var req AdminUpdateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if !req.hasGeneralUpdates() {
		h.updateGroupAndRateLimit(c, keyID, req)
		return
	}
	if h.apiKeyService == nil {
		response.InternalError(c, "API key service unavailable")
		return
	}
	current, err := h.apiKeyService.GetByID(c.Request.Context(), keyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	svcReq, err := adminUpdateAPIKeyServiceRequest(req, current)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	key, err := h.apiKeyService.Update(c.Request.Context(), keyID, current.UserID, svcReq)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	var result *service.AdminUpdateAPIKeyGroupIDResult
	if req.GroupID != nil {
		result, err = h.adminService.AdminUpdateAPIKeyGroupID(c.Request.Context(), keyID, req.GroupID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		key = result.APIKey
	}

	resp := struct {
		APIKey                 *dto.APIKey `json:"api_key"`
		AutoGrantedGroupAccess bool        `json:"auto_granted_group_access"`
		GrantedGroupID         *int64      `json:"granted_group_id,omitempty"`
		GrantedGroupName       string      `json:"granted_group_name,omitempty"`
	}{
		APIKey: dto.APIKeyFromService(key),
	}
	if result != nil {
		resp.AutoGrantedGroupAccess = result.AutoGrantedGroupAccess
		resp.GrantedGroupID = result.GrantedGroupID
		resp.GrantedGroupName = result.GrantedGroupName
	}
	response.Success(c, resp)
}

// Delete handles deleting any user's API key.
// DELETE /api/v1/admin/api-keys/:id
func (h *AdminAPIKeyHandler) Delete(c *gin.Context) {
	if h.apiKeyService == nil {
		response.InternalError(c, "API key service unavailable")
		return
	}
	keyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid API key ID")
		return
	}
	key, err := h.apiKeyService.GetByID(c.Request.Context(), keyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.apiKeyService.Delete(c.Request.Context(), keyID, key.UserID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "API key deleted successfully"})
}

func (h *AdminAPIKeyHandler) updateGroupAndRateLimit(c *gin.Context, keyID int64, req AdminUpdateAPIKeyRequest) {
	var err error
	var resetKey *service.APIKey
	if req.ResetRateLimitUsage != nil && *req.ResetRateLimitUsage {
		resetKey, err = h.adminService.AdminResetAPIKeyRateLimitUsage(c.Request.Context(), keyID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}

	result, err := h.adminService.AdminUpdateAPIKeyGroupID(c.Request.Context(), keyID, req.GroupID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if resetKey != nil && req.GroupID == nil {
		result.APIKey = resetKey
	}

	resp := struct {
		APIKey                 *dto.APIKey `json:"api_key"`
		AutoGrantedGroupAccess bool        `json:"auto_granted_group_access"`
		GrantedGroupID         *int64      `json:"granted_group_id,omitempty"`
		GrantedGroupName       string      `json:"granted_group_name,omitempty"`
	}{
		APIKey:                 dto.APIKeyFromService(result.APIKey),
		AutoGrantedGroupAccess: result.AutoGrantedGroupAccess,
		GrantedGroupID:         result.GrantedGroupID,
		GrantedGroupName:       result.GrantedGroupName,
	}
	response.Success(c, resp)
}

func (req AdminUpdateAPIKeyRequest) hasGeneralUpdates() bool {
	return req.Name != "" ||
		req.Status != "" ||
		req.IPWhitelist != nil ||
		req.IPBlacklist != nil ||
		req.Quota != nil ||
		req.ExpiresAt != nil ||
		req.ResetQuota != nil ||
		req.RateLimit5h != nil ||
		req.RateLimit1d != nil ||
		req.RateLimit7d != nil ||
		req.Concurrency != nil
}

func adminUpdateAPIKeyServiceRequest(req AdminUpdateAPIKeyRequest, current *service.APIKey) (service.UpdateAPIKeyRequest, error) {
	svcReq := service.UpdateAPIKeyRequest{
		Quota:               req.Quota,
		ResetQuota:          req.ResetQuota,
		RateLimit5h:         req.RateLimit5h,
		RateLimit1d:         req.RateLimit1d,
		RateLimit7d:         req.RateLimit7d,
		ResetRateLimitUsage: req.ResetRateLimitUsage,
		Concurrency:         req.Concurrency,
	}
	if req.IPWhitelist != nil {
		svcReq.IPWhitelist = req.IPWhitelist
	} else if current != nil {
		svcReq.IPWhitelist = current.IPWhitelist
	}
	if req.IPBlacklist != nil {
		svcReq.IPBlacklist = req.IPBlacklist
	} else if current != nil {
		svcReq.IPBlacklist = current.IPBlacklist
	}
	if req.Name != "" {
		svcReq.Name = &req.Name
	}
	if req.Status != "" {
		svcReq.Status = &req.Status
	}
	if req.ExpiresAt != nil {
		if *req.ExpiresAt == "" {
			svcReq.ClearExpiration = true
		} else {
			expiresAt, err := time.Parse(time.RFC3339, *req.ExpiresAt)
			if err != nil {
				return service.UpdateAPIKeyRequest{}, err
			}
			svcReq.ExpiresAt = &expiresAt
		}
	}
	return svcReq, nil
}
