package workinsight

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

type AdminHandler struct{ service *Service }

func NewAdminHandler(service *Service) *AdminHandler { return &AdminHandler{service: service} }

func (h *AdminHandler) GetConfig(c *gin.Context) { response.Success(c, h.service.PublicConfig()) }

func (h *AdminHandler) UpdateConfig(c *gin.Context) {
	var req Config
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "AI 使用洞察配置请求无效")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "管理员身份无效")
		return
	}
	result, err := h.service.SaveConfig(c.Request.Context(), req, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *AdminHandler) GetRuntime(c *gin.Context) {
	runtime, err := h.service.Runtime(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, runtime)
}

func (h *AdminHandler) AnalyzeNow(c *gin.Context) {
	created, err := h.service.AnalyzeNow(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"created_batches": created})
}

func (h *AdminHandler) RetryBatch(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "分析批次 ID 无效")
		return
	}
	if err := h.service.RetryBatchNow(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"batch_id": id})
}

func (h *AdminHandler) RetryAllBatches(c *gin.Context) {
	result, err := h.service.RetryAllBatchesNow(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *AdminHandler) StopBatch(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "分析批次 ID 无效")
		return
	}
	if err := h.service.StopBatchNow(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"batch_id": id})
}

func (h *AdminHandler) DeleteFailedBatch(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "分析批次 ID 无效")
		return
	}
	if err := h.service.DeleteFailedBatchNow(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"batch_id": id})
}

func (h *AdminHandler) DeleteAllFailedBatches(c *gin.Context) {
	deleted, err := h.service.DeleteAllFailedBatchesNow(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"batches": deleted})
}

func (h *AdminHandler) ListBatches(c *gin.Context) {
	page, size := response.ParsePagination(c)
	items, total, err := h.service.repo.ListBatches(c.Request.Context(), page, size, c.DefaultQuery("kind", "done"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}

func (h *AdminHandler) ClearLogs(c *gin.Context) {
	result, err := h.service.repo.ClearTerminalLogs(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *AdminHandler) ListSamples(c *gin.Context) {
	page, size := response.ParsePagination(c)
	items, total, err := h.service.repo.ListSamples(c.Request.Context(), page, size)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}

func (h *AdminHandler) GetSample(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "样本 ID 无效")
		return
	}
	item, err := h.service.repo.GetSample(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if item == nil {
		response.NotFound(c, "样本不存在")
		return
	}
	response.Success(c, item)
}

func (h *AdminHandler) ListAnalyzerAccounts(c *gin.Context) {
	items, err := h.service.AnalyzerAccounts(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *AdminHandler) Probe(c *gin.Context) {
	var req Config
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "分析节点探测请求无效")
		return
	}
	response.Success(c, h.service.Probe(c.Request.Context(), req))
}

func (h *AdminHandler) ListDaily(c *gin.Context) {
	page, size := response.ParsePagination(c)
	filter, ok := parseDailyFilter(c)
	if !ok {
		return
	}
	filter.Page, filter.Size = page, size
	items, total, err := h.service.repo.ListDaily(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}

func (h *AdminHandler) ListRanking(c *gin.Context) {
	page, size := response.ParsePagination(c)
	filter, ok := parseDailyFilter(c)
	if !ok {
		return
	}
	filter.Page, filter.Size = page, size
	items, total, err := h.service.repo.ListUserRanking(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}

func (h *AdminHandler) GetOverview(c *gin.Context) {
	filter, ok := parseDailyFilter(c)
	if !ok {
		return
	}
	result, err := h.service.repo.Overview(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func parseDailyFilter(c *gin.Context) (DailyFilter, bool) {
	search := c.Query("user_name")
	if search == "" {
		search = c.Query("search")
	}
	category := c.Query("task_category")
	if category == "" {
		category = c.Query("category")
	}
	filter := DailyFilter{Search: search, Category: category, Project: c.Query("project_name")}
	var err error
	if value := strings.TrimSpace(c.Query("start_date")); value != "" {
		filter.Start, err = time.Parse("2006-01-02", value)
		if err != nil {
			response.BadRequest(c, "开始日期无效")
			return DailyFilter{}, false
		}
	}
	if value := strings.TrimSpace(c.Query("end_date")); value != "" {
		filter.End, err = time.Parse("2006-01-02", value)
		if err != nil {
			response.BadRequest(c, "结束日期无效")
			return DailyFilter{}, false
		}
	}
	if filter.Category != "" && !contains(TaskCategories, filter.Category) {
		response.BadRequest(c, "任务类型无效")
		return DailyFilter{}, false
	}
	return filter, true
}

func (h *AdminHandler) GetDaily(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "洞察 ID 无效")
		return
	}
	item, err := h.service.repo.GetDaily(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if item == nil {
		response.NotFound(c, "洞察不存在")
		return
	}
	samples, total, err := h.service.repo.ListRepresentativeItems(c.Request.Context(), item.UserID, item.Date, 20, 0)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"insight": item, "representative_items": samples, "representative_item_count": total,
		"representative_items_expired": total == 0 && time.Since(item.Date) > time.Duration(h.service.PublicConfig().SampleRetentionDays)*24*time.Hour,
	})
}

func (h *AdminHandler) ListRepresentativeItems(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "洞察 ID 无效")
		return
	}
	item, err := h.service.repo.GetDaily(c.Request.Context(), id)
	if err != nil || item == nil {
		if err != nil {
			response.ErrorFrom(c, err)
		} else {
			response.NotFound(c, "洞察不存在")
		}
		return
	}
	page, size := response.ParsePagination(c)
	items, total, err := h.service.repo.ListRepresentativeItems(c.Request.Context(), item.UserID, item.Date, size, (page-1)*size)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
