package admin

import (
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type CostManagementHandler struct {
	service *service.CostManagementService
}

func NewCostManagementHandler(s *service.CostManagementService) *CostManagementHandler {
	return &CostManagementHandler{service: s}
}

func (h *CostManagementHandler) ListPlans(c *gin.Context) {
	page, size := response.ParsePagination(c)
	items, total, err := h.service.ListPlans(c.Request.Context(), page, size, c.Query("type"), c.Query("search"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}
func (h *CostManagementHandler) GetPlan(c *gin.Context) {
	id, ok := costID(c)
	if !ok {
		return
	}
	x, err := h.service.GetPlan(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, x)
}
func (h *CostManagementHandler) CreatePlan(c *gin.Context) {
	var in service.CostPlanInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	x, err := h.service.CreatePlan(c.Request.Context(), in)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, x)
}
func (h *CostManagementHandler) UpdatePlan(c *gin.Context) {
	id, ok := costID(c)
	if !ok {
		return
	}
	var in service.CostPlanBasicInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	x, err := h.service.UpdatePlan(c.Request.Context(), id, in)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, x)
}
func (h *CostManagementHandler) ChangePlanPrice(c *gin.Context) {
	id, ok := costID(c)
	if !ok {
		return
	}
	var in service.CostPriceChangeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.service.ChangePlanPrice(c.Request.Context(), id, in); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"saved": true})
}
func (h *CostManagementHandler) ListPriceHistory(c *gin.Context) {
	id, ok := costID(c)
	if !ok {
		return
	}
	items, err := h.service.ListPriceHistory(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}
func (h *CostManagementHandler) DisablePlan(c *gin.Context) {
	id, ok := costID(c)
	if !ok {
		return
	}
	if err := h.service.DisablePlan(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"disabled": true})
}
func (h *CostManagementHandler) ListAccounts(c *gin.Context) {
	page, size := response.ParsePagination(c)
	items, total, err := h.service.ListAccounts(c.Request.Context(), page, size, c.Query("mode"), c.Query("search"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}
func (h *CostManagementHandler) ListModelOptions(c *gin.Context) {
	page, size := response.ParsePagination(c)
	items, total, err := h.service.ListModelOptions(c.Request.Context(), page, size, c.Query("search"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}
func (h *CostManagementHandler) ListSubscriptionUnits(c *gin.Context) {
	planID, err := strconv.ParseInt(c.Query("plan_id"), 10, 64)
	if err != nil || planID <= 0 {
		response.BadRequest(c, "invalid plan_id")
		return
	}
	items, err := h.service.ListSubscriptionUnits(c.Request.Context(), planID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}
func (h *CostManagementHandler) CreateSubscriptionUnit(c *gin.Context) {
	var req service.CostSubscriptionUnitInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	item, err := h.service.CreateSubscriptionUnit(c.Request.Context(), req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, item)
}
func (h *CostManagementHandler) RenameSubscriptionUnit(c *gin.Context) {
	id, ok := costID(c)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.service.RenameSubscriptionUnit(c.Request.Context(), id, req.Name); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"renamed": true})
}
func (h *CostManagementHandler) EndSubscriptionUnit(c *gin.Context) {
	id, ok := costID(c)
	if !ok {
		return
	}
	var req struct {
		EffectiveTo time.Time `json:"effective_to"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.service.EndSubscriptionUnit(c.Request.Context(), id, req.EffectiveTo); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"ended": true})
}
func (h *CostManagementHandler) SaveAccount(c *gin.Context) {
	id, ok := costID(c)
	if !ok {
		return
	}
	var in service.AccountCostInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	in.AccountID = id
	if err := h.service.SaveAccount(c.Request.Context(), in); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"saved": true})
}
func (h *CostManagementHandler) SaveAccounts(c *gin.Context) {
	var req struct {
		AccountIDs []int64 `json:"account_ids"`
		service.AccountCostInput
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.AccountIDs) == 0 {
		response.BadRequest(c, "account_ids are required")
		return
	}
	inputs := make([]service.AccountCostInput, 0, len(req.AccountIDs))
	for _, id := range req.AccountIDs {
		in := req.AccountCostInput
		in.AccountID = id
		inputs = append(inputs, in)
	}
	if err := h.service.SaveAccounts(c.Request.Context(), inputs); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"saved": len(req.AccountIDs)})
}
func (h *CostManagementHandler) EndAccount(c *gin.Context) {
	id, ok := costID(c)
	if !ok {
		return
	}
	if err := h.service.EndAccount(c.Request.Context(), id, time.Now()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"ended": true})
}
func (h *CostManagementHandler) Overview(c *gin.Context) {
	start, end, ok := costDateRange(c)
	if !ok {
		return
	}
	x, err := h.service.Overview(c.Request.Context(), start, end)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, x)
}
func (h *CostManagementHandler) Analysis(c *gin.Context) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	x, err := h.service.Analysis(c.Request.Context(), c.DefaultQuery("period", "day"), time.Now().In(loc))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, x)
}
func (h *CostManagementHandler) UserCosts(c *gin.Context) {
	start, end, ok := costDateRange(c)
	if !ok {
		return
	}
	x, err := h.service.UserCosts(c.Request.Context(), start, end)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, x)
}
func (h *CostManagementHandler) ListRecalculations(c *gin.Context) {
	page, size := response.ParsePagination(c)
	x, total, err := h.service.ListJobs(c.Request.Context(), page, size)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, x, total, page, size)
}
func (h *CostManagementHandler) CreateRecalculation(c *gin.Context) {
	var req struct {
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	start, err := time.ParseInLocation("2006-01-02", req.StartDate, loc)
	if err != nil {
		response.BadRequest(c, "invalid start_date")
		return
	}
	end, err := time.ParseInLocation("2006-01-02", req.EndDate, loc)
	if err != nil {
		response.BadRequest(c, "invalid end_date")
		return
	}
	var uid int64
	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok {
		uid = subject.UserID
	}
	x, err := h.service.CreateRecalculation(c.Request.Context(), start, end, uid)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Accepted(c, x)
}
func (h *CostManagementHandler) CancelRecalculation(c *gin.Context) {
	id, ok := costID(c)
	if !ok {
		return
	}
	if err := h.service.CancelRecalculation(c.Request.Context(), id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"message": "核算任务已取消"})
}

func costID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid id")
		return 0, false
	}
	return id, true
}
func costDateRange(c *gin.Context) (time.Time, time.Time, bool) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	start, err := time.ParseInLocation("2006-01-02", c.Query("start_date"), loc)
	if err != nil {
		response.BadRequest(c, "invalid start_date")
		return time.Time{}, time.Time{}, false
	}
	end, err := time.ParseInLocation("2006-01-02", c.Query("end_date"), loc)
	if err != nil || end.Before(start) {
		response.BadRequest(c, "invalid end_date")
		return time.Time{}, time.Time{}, false
	}
	return start, end.AddDate(0, 0, 1), true
}
