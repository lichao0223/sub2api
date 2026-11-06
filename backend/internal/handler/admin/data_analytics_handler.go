package admin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type DataAnalyticsHandler struct{ service *service.DataAnalyticsService }

func NewDataAnalyticsHandler(s *service.DataAnalyticsService) *DataAnalyticsHandler {
	return &DataAnalyticsHandler{service: s}
}

func analyticsError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrAnalyticsControlConflict) {
		response.Error(c, http.StatusConflict, err.Error())
	} else if errors.Is(err, service.ErrAnalyticsTasksPaused) {
		response.Error(c, http.StatusConflict, "统计已暂停，请先在系统设置开启后台统计任务")
	} else {
		response.BadRequest(c, err.Error())
	}
}

func analyticsActor(c *gin.Context) (int64, bool) {
	s, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || s.UserID <= 0 {
		response.Unauthorized(c, "authentication required")
		return 0, false
	}
	return s.UserID, true
}

func (h *DataAnalyticsHandler) Directory(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	x, err := h.service.Directory(ctx)
	if err != nil {
		analyticsError(c, err)
		return
	}
	response.Success(c, x)
}

func (h *DataAnalyticsHandler) Preview(c *gin.Context) {
	actor, ok := analyticsActor(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2*service.AnalyticsSQLLimit+(1<<20))
	if err := c.Request.ParseMultipartForm(2 << 20); err != nil {
		response.BadRequest(c, "请上传两份20MiB以内的SQL文件")
		return
	}
	defer func() {
		if c.Request.MultipartForm != nil {
			_ = c.Request.MultipartForm.RemoveAll()
		}
	}()
	read := func(name string) (string, error) {
		f, head, err := c.Request.FormFile(name)
		if err != nil {
			return "", errors.New("必须同时上传组织SQL和人员SQL")
		}
		defer func() { _ = f.Close() }()
		if !strings.HasSuffix(strings.ToLower(head.Filename), ".sql") {
			return "", errors.New("只允许.sql文件")
		}
		data, err := io.ReadAll(io.LimitReader(f, service.AnalyticsSQLLimit+1))
		if err != nil {
			return "", err
		}
		if len(data) > service.AnalyticsSQLLimit {
			return "", errors.New("SQL文件超过20MiB")
		}
		return string(data), nil
	}
	org, err := read("organizations")
	if err != nil {
		analyticsError(c, err)
		return
	}
	users, err := read("people")
	if err != nil {
		analyticsError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	x, err := h.service.Preview(ctx, org, users, actor)
	if err != nil {
		analyticsError(c, err)
		return
	}
	response.Success(c, x)
}

func (h *DataAnalyticsHandler) Apply(c *gin.Context) {
	actor, ok := analyticsActor(c)
	if !ok {
		return
	}
	var in struct {
		ID        string    `json:"id" binding:"required"`
		Effective time.Time `json:"effective_at" binding:"required"`
		Version   int64     `json:"version"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "预检ID和生效时间不能为空")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	if err := h.service.Apply(ctx, in.ID, in.Effective, in.Version, actor); err != nil {
		analyticsError(c, err)
		return
	}
	response.Success(c, gin.H{"applied": true})
}

func (h *DataAnalyticsHandler) Map(c *gin.Context) {
	actor, ok := analyticsActor(c)
	if !ok {
		return
	}
	var in struct {
		User    int64  `json:"user_id" binding:"required,min=1"`
		Person  string `json:"person_id" binding:"required"`
		Version int64  `json:"version"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "请选择平台账号和目录人员")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	if err := h.service.Map(ctx, in.User, in.Person, in.Version, actor); err != nil {
		analyticsError(c, err)
		return
	}
	response.Success(c, gin.H{"saved": true})
}

func (h *DataAnalyticsHandler) Report(c *gin.Context) {
	now := time.Now()
	year, err := strconv.Atoi(c.DefaultQuery("year", strconv.Itoa(now.Year())))
	if err != nil {
		response.BadRequest(c, "年份无效")
		return
	}
	var months []int
	if text := c.Query("months"); text != "" {
		for _, s := range strings.Split(text, ",") {
			m, e := strconv.Atoi(s)
			if e != nil {
				response.BadRequest(c, "月份无效")
				return
			}
			months = append(months, m)
		}
	}
	period, err := service.NewAnalyticsPeriod(year, c.DefaultQuery("mode", "year"), months, now)
	if err != nil {
		analyticsError(c, err)
		return
	}
	page, size := response.ParsePagination(c)
	if size > 100 {
		size = 100
	}
	q := service.AnalyticsQuery{Period: period, OrgID: c.Query("org_id"), Search: strings.TrimSpace(c.Query("search")), Page: page, PageSize: size, Sort: c.DefaultQuery("sort", "tokens"), Export: c.Query("export") == "true"}
	if len(q.Search) > 512 || len(q.OrgID) > 128 || (q.Sort != "tokens" && q.Sort != "cost" && q.Sort != "name") {
		response.BadRequest(c, "筛选条件无效")
		return
	}
	if id := c.Query("user_id"); id != "" {
		q.UserID, err = strconv.ParseInt(id, 10, 64)
		if err != nil || q.UserID <= 0 {
			response.BadRequest(c, "人员ID无效")
			return
		}
	}
	if versions := c.Query("versions"); versions != "" {
		if len(versions) > 2048 || json.Unmarshal([]byte(versions), &q.Versions) != nil {
			response.BadRequest(c, "发布版本无效")
			return
		}
	}
	if q.Export {
		q.Search = ""
		q.Page = 1
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	x, err := h.service.Report(ctx, q)
	if err != nil {
		analyticsError(c, err)
		return
	}
	if q.Export {
		data, err := json.Marshal(x)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		if len(data) > 20<<20 {
			response.BadRequest(c, "报表超过20MiB，请按部门或月份拆分")
			return
		}
	}
	response.Success(c, x)
}

func (h *DataAnalyticsHandler) Rebuild(c *gin.Context) {
	actor, ok := analyticsActor(c)
	if !ok {
		return
	}
	var in struct {
		Start string `json:"start" binding:"required"`
		End   string `json:"end" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "请选择补算日期")
		return
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	start, e1 := time.ParseInLocation("2006-01-02", in.Start, loc)
	end, e2 := time.ParseInLocation("2006-01-02", in.End, loc)
	if e1 != nil || e2 != nil {
		response.BadRequest(c, "日期格式无效")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := h.service.Rebuild(ctx, start, end, actor); err != nil {
		analyticsError(c, err)
		return
	}
	response.Accepted(c, gin.H{"queued": true})
}
