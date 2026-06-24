# Token 使用排名非工作时段设计

## 背景

当前 Token 使用排名按时间范围聚合用户的请求数、Token 数和消费。新增需求是看出哪些用户在非工作日或工作日下班时段仍在使用系统，以及用了多少 Token、发起多少请求、产生多少消费、持续活跃了多久。

这里的目标不是做风控判定，也不应使用“异常”这样的带判断色彩的命名。产品口径统一使用：

- 非工作日：法定节假日、调休休息日、普通周末。
- 工作日下班时段：工作日工作时间之外，例如默认 `08:30` 前和 `18:00` 后。
- 非工作时段：非工作日全天 + 工作日下班时段。
- 活跃时长：根据请求时间序列会话化计算的持续使用时间，不等同于 AI 回复耗时、请求响应耗时、用户在线时长或 IDE 打开时长。

## 设计原则

1. 节假日数据必须入库，业务查询只读本地库，不在排名接口里实时请求第三方服务。
2. 每个自然日都应该有日历记录，包括普通工作日和普通周末。
3. 自动来源只作为同步输入；入库后的版本、来源和确认状态要可审计。
4. 历史排名不能依赖原始 `usage_logs` 永久存在，必须有面向该报表的聚合表。
5. 所有时间判断以 `Asia/Shanghai` 为准，不使用服务器本地时区，也不使用 UTC 日期直接判断工作日。
6. 活跃时长按请求间隔算法计算，不能直接用 `duration_ms` 作为用户活跃时间。

## 数据来源

推荐自动来源为 `holiday-cn`：

```text
https://raw.githubusercontent.com/NateScarlet/holiday-cn/master/{year}.json
```

该数据用于覆盖官方节假日休息日和调休上班日。普通周末和普通工作日由系统按星期规则生成。

数据源优先级：

1. 管理员手工修正：最高优先级，用于处理临时通知、地区性要求或数据源错误。
2. `holiday-cn` 年度 JSON：自动来源。
3. 默认星期规则：当某年份官方安排尚未发布时使用。

建议保留官方公告链接或数据源版本信息，方便后续审计。国务院办公厅公告是权威来源，`holiday-cn` 作为自动化同步来源。

## 日历维表

每一天都入库，不只存节假日。

```sql
CREATE TABLE calendar_days (
  date DATE NOT NULL,
  country VARCHAR(8) NOT NULL DEFAULT 'CN',
  is_workday BOOLEAN NOT NULL,
  is_offday BOOLEAN NOT NULL,
  is_weekend BOOLEAN NOT NULL,
  day_type VARCHAR(32) NOT NULL,
  holiday_name VARCHAR(64),
  source VARCHAR(64) NOT NULL,
  source_version VARCHAR(128),
  confirmed BOOLEAN NOT NULL DEFAULT false,
  manual_override BOOLEAN NOT NULL DEFAULT false,
  raw JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (country, date)
);

CREATE INDEX idx_calendar_days_country_confirmed
  ON calendar_days (country, confirmed);
```

`day_type` 建议枚举：

```text
normal_workday       普通工作日
normal_weekend       普通周末
holiday_offday       节假日/调休休息日
makeup_workday       调休上班日
manual_workday       手工指定工作日
manual_offday        手工指定休息日
predicted_workday    未确认年份的预测工作日
predicted_weekend    未确认年份的预测周末
```

字段口径：

- `is_workday=true` 表示这天按工作日处理。
- `is_offday=true` 表示这天按休息日处理。
- `is_workday` 和 `is_offday` 应始终互斥。
- `is_weekend` 只表示自然周六/周日，不代表一定休息，因为调休上班日可能是周末。
- `confirmed=false` 表示该日期来自默认星期规则或未确认来源，未来可能被官方安排覆盖。
- `manual_override=true` 的记录同步任务不能覆盖。

## 同步任务

新增日历同步服务 `CalendarSyncService`。

任务职责：

1. 每天定时同步当前年份和下一年份。
2. 每年 10 月到次年 1 月可提高同步频率，因为下一年公告通常在这个窗口发布，但不能依赖固定日期。
3. 若远端年度 JSON 不存在或读取失败，生成/保留默认星期规则。
4. 若远端数据变化，重新生成该年份日历并 upsert。
5. 记录同步运行日志，包括年份、来源、版本、变更数量、失败原因。

同步日志表：

```sql
CREATE TABLE calendar_sync_runs (
  id BIGSERIAL PRIMARY KEY,
  country VARCHAR(8) NOT NULL DEFAULT 'CN',
  year INT NOT NULL,
  source VARCHAR(64) NOT NULL,
  source_url TEXT,
  source_version VARCHAR(128),
  status VARCHAR(32) NOT NULL,
  days_inserted INT NOT NULL DEFAULT 0,
  days_updated INT NOT NULL DEFAULT 0,
  error_message TEXT,
  started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  finished_at TIMESTAMPTZ
);

CREATE INDEX idx_calendar_sync_runs_year
  ON calendar_sync_runs (country, year, started_at DESC);
```

## 年度日历生成算法

输入：

- `year`
- `country = CN`
- `holiday-cn/{year}.json` 的 `days`

步骤：

1. 生成全年所有日期。
2. 对每一天按星期生成默认值：
   - 周一到周五：`is_workday=true`，`day_type=normal_workday`。
   - 周六周日：`is_workday=false`，`day_type=normal_weekend`。
3. 读取自动来源 `days` 并覆盖：
   - `isOffDay=true`：`is_workday=false`，`day_type=holiday_offday`，写入 `holiday_name`。
   - `isOffDay=false`：`is_workday=true`，`day_type=makeup_workday`，写入 `holiday_name`。
4. 如果自动来源不存在：
   - 周一到周五：`day_type=predicted_workday`。
   - 周六周日：`day_type=predicted_weekend`。
   - `confirmed=false`。
5. 如果自动来源存在并解析成功：
   - 被默认规则生成且没有出现在自动来源的日期也可以设为 `confirmed=true`，表示该年份已有官方安排覆盖。
6. upsert 时跳过 `manual_override=true` 的记录。

伪代码：

```go
func BuildCalendar(year int, sourceDays []HolidayDay, confirmed bool) []CalendarDay {
    days := generateEveryDateOfYear(year)
    result := map[date]CalendarDay{}

    for _, d := range days {
        weekend := d.Weekday() == time.Saturday || d.Weekday() == time.Sunday
        if weekend {
            result[d] = CalendarDay{Date: d, IsWorkday: false, IsOffday: true, IsWeekend: true, DayType: "normal_weekend", Confirmed: confirmed}
        } else {
            result[d] = CalendarDay{Date: d, IsWorkday: true, IsOffday: false, IsWeekend: false, DayType: "normal_workday", Confirmed: confirmed}
        }
    }

    if !confirmed {
        markPredicted(result)
        return values(result)
    }

    for _, h := range sourceDays {
        d := parseDate(h.Date)
        current := result[d]
        current.HolidayName = h.Name
        current.Source = "holiday-cn"
        current.Raw = h.Raw
        if h.IsOffDay {
            current.IsWorkday = false
            current.IsOffday = true
            current.DayType = "holiday_offday"
        } else {
            current.IsWorkday = true
            current.IsOffday = false
            current.DayType = "makeup_workday"
        }
        result[d] = current
    }

    return values(result)
}
```

## 明年数据怎么来

对于下一年数据，不要等用户查询时报错。

策略：

1. 系统启动或每日任务主动生成下一年默认星期规则，`confirmed=false`。
2. 同步任务每天尝试拉取 `holiday-cn/{next_year}.json`。
3. 如果拉取成功且数据有效，覆盖下一年日历，`confirmed=true`。
4. 管理后台显示日历状态：
   - `2027 年日历：未确认，按周末规则预测`
   - `2027 年日历：已确认，来源 holiday-cn`
5. 查询接口返回 `calendar_confirmed`，前端可提示用户当前数据是否为预测。

## 非工作时段判定

所有 `usage_logs.created_at` 先转换到 `Asia/Shanghai`：

```text
local_time = created_at AT TIME ZONE 'Asia/Shanghai'
local_date = local_time.date
local_clock = local_time.time
```

判定：

```text
calendar_days.is_offday = true
  -> 非工作日

calendar_days.is_workday = true AND local_clock < work_start
  -> 工作日下班时段

calendar_days.is_workday = true AND local_clock >= work_end
  -> 工作日下班时段

其他
  -> 工作日正常时段
```

默认工作时间：

```text
work_start = 08:30
work_end = 18:00
timezone = Asia/Shanghai
```

这三个值应该配置化，至少放在后端配置中。后续若需要企业级能力，可支持按用户组配置不同工作时间。

## 活跃时长算法

### 为什么不用 duration_ms

`duration_ms` 表示单次 API 请求处理/响应耗时。它适合做性能分析，但不适合表示用户持续工作时间。

例如用户 20:00 发一个请求，20:04 又发一个请求，两次请求各自只耗时 5 秒。如果只加 `duration_ms`，只能得到 10 秒；但从使用行为看，用户在这 4 分钟里大概率持续围绕这项工作。

### 推荐口径

按用户请求时间序列计算“活跃会话”：

1. 同一用户的请求按 `created_at` 升序排序。
2. 相邻两次请求间隔 `<= active_gap_minutes`，把这段间隔计入活跃时长。
3. 相邻两次请求间隔 `> active_gap_minutes`，上一段会话结束，下一次请求开启新会话。
4. 单次孤立请求默认计 `min_session_minutes`，建议默认 1 分钟。
5. 默认配置：
   - `active_gap_minutes = 5`
   - `min_session_minutes = 1`
6. 跨越工作/非工作边界的间隔按分钟或秒级切片分摊。

示例：

```text
08:00 请求
08:03 请求  -> +3 分钟
08:10 请求  -> 间隔 7 分钟，断开
08:13 请求  -> +3 分钟
```

基础活跃时长为 6 分钟。孤立请求如何补最小值取决于会话实现：

- 如果会话内已有连续间隔，不额外补每个请求。
- 如果一个会话只有 1 个请求，补 `min_session_minutes`。

### 会话化伪代码

```go
type RequestPoint struct {
    UserID    int64
    CreatedAt time.Time // UTC
    Tokens    int64
    Cost      float64
}

func BuildActiveSegments(points []RequestPoint, gap time.Duration, minSession time.Duration) []ActiveSegment {
    sort by UserID, CreatedAt

    var segments []ActiveSegment
    var sessionStart, last time.Time
    var currentUser int64
    var requestCount int

    flush := func() {
        if requestCount == 0 {
            return
        }
        end := last
        if requestCount == 1 {
            end = sessionStart.Add(minSession)
        }
        if end.After(sessionStart) {
            segments = append(segments, ActiveSegment{
                UserID: currentUser,
                Start: sessionStart,
                End: end,
            })
        }
    }

    for _, p := range points {
        if requestCount == 0 || p.UserID != currentUser {
            flush()
            currentUser = p.UserID
            sessionStart = p.CreatedAt
            last = p.CreatedAt
            requestCount = 1
            continue
        }

        if p.CreatedAt.Sub(last) <= gap {
            last = p.CreatedAt
            requestCount++
            continue
        }

        flush()
        sessionStart = p.CreatedAt
        last = p.CreatedAt
        requestCount = 1
    }
    flush()

    return segments
}
```

### 时段分摊

活跃段可能跨日期、跨 08:30、跨 18:00、跨节假日边界，不能简单按起点归类。

应将 `[segment_start, segment_end)` 切成多个子段，切点包括：

- 每天 00:00
- 每天 `work_start`
- 每天 `work_end`
- 会话开始和结束

每个子段查 `calendar_days` 后归类：

```text
offday_active_ms
after_hours_active_ms
work_hours_active_ms
```

这样 `17:58 - 18:03` 会被拆成：

```text
17:58 - 18:00 工作日正常时段 2 分钟
18:00 - 18:03 工作日下班时段 3 分钟
```

### 容易错的点

- 不要把两个相距 30 分钟的请求之间都算作活跃。
- 不要给每个请求都加 1 分钟，否则高频请求会被重复放大。
- 不要按 UTC 日期判断节假日。
- 不要只按请求开始时间归类跨边界会话。
- 不要把 `duration_ms` 和活跃时长混为一谈。
- 对同一个 request_id 的重复写入或重试记录要去重，否则请求数和活跃时长都会偏大。

## 聚合表设计

如果只靠 `usage_logs` 临时计算，历史会受到保留策略影响。当前配置中原始 `usage_logs` 可能只保留 90 天，因此需要新增面向排名的聚合表。

建议按天、用户、时段类型聚合：

```sql
CREATE TABLE usage_nonwork_daily_user_stats (
  bucket_date DATE NOT NULL,
  timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai',
  user_id BIGINT NOT NULL,
  segment VARCHAR(32) NOT NULL,

  requests BIGINT NOT NULL DEFAULT 0,
  input_tokens BIGINT NOT NULL DEFAULT 0,
  output_tokens BIGINT NOT NULL DEFAULT 0,
  cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
  cache_read_tokens BIGINT NOT NULL DEFAULT 0,
  total_tokens BIGINT NOT NULL DEFAULT 0,
  actual_cost DECIMAL(20, 10) NOT NULL DEFAULT 0,

  active_ms BIGINT NOT NULL DEFAULT 0,
  active_sessions BIGINT NOT NULL DEFAULT 0,

  calendar_confirmed BOOLEAN NOT NULL DEFAULT true,
  computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (bucket_date, timezone, user_id, segment)
);

CREATE INDEX idx_usage_nonwork_daily_user_stats_user_date
  ON usage_nonwork_daily_user_stats (user_id, bucket_date DESC);

CREATE INDEX idx_usage_nonwork_daily_user_stats_segment_date
  ON usage_nonwork_daily_user_stats (segment, bucket_date DESC);
```

`segment` 建议枚举：

```text
work_hours       工作日正常时段
after_hours      工作日下班时段
offday           非工作日
```

报表查询“非工作时段”时聚合：

```text
segment IN ('after_hours', 'offday')
```

## 聚合任务

新增 `UsageNonworkAggregationService`。

任务类型：

1. 增量聚合：每 5 到 15 分钟跑一次，处理最近 N 小时。
2. 近期重算：每天重算最近 2 到 3 天，修正延迟写入、流式请求完成延迟、日历更新。
3. 手动回填：管理员触发指定日期范围回填。
4. 日历变更重算：某年 `calendar_days` 从未确认变为确认，或被手工修正后，重算受影响日期。

重要策略：

- 聚合任务应该可重入。先删除目标日期范围的聚合结果，再重新插入。
- 活跃时长算法需要查询范围前后各扩展 `active_gap_minutes`，否则边界上的连续会话会被截断。
- 按用户计算活跃段时，必须按完整时间序列排序，不能只在 SQL group by 中粗略求差。

## 查询接口设计

扩展现有 Token 排名接口，或新增更明确接口。

推荐新增：

```text
GET /api/v1/usage/dashboard/token-ranking/nonwork
```

参数：

```text
start_date=2026-06-18
end_date=2026-06-24
scope=nonwork|all
rank_by=nonwork_tokens|requests|active_duration|actual_cost
limit=50
timezone=Asia/Shanghai
work_start=08:30
work_end=18:00
```

返回：

```json
{
  "ranking": [
    {
      "user_id": 95,
      "email": "xu.man@example.com",
	      "username": "许曼",
	      "requests": 142,
	      "tokens": 2218600,
	      "nonwork_tokens": 2218600,
	      "active_duration_ms": 36060000,
	      "actual_cost": 31.68
	    }
	  ],
	  "total_requests": 52809,
	  "total_tokens": 7410000000,
	  "total_nonwork_tokens": 7410000000,
	  "total_all_tokens": 14820000000,
	  "nonwork_token_ratio": 0.5,
	  "total_active_duration_ms": 128000000,
	  "total_actual_cost": 6110.0,
  "calendar_confirmed": true,
  "start_date": "2026-06-18",
  "end_date": "2026-06-24"
}
```

排序规则：

```text
rank_by 主排序 DESC
tokens DESC
active_duration_ms DESC
user_id ASC
```

## 前端设计

基于现有 Token 使用排名页面，不做新页面风格。

筛选区：

- 时间范围：保持原有 DateRangePicker 样式，例如 `近 7 天`。
- 排名维度：
  - 非工作时间 Token
  - 请求数
  - 活跃时长
  - 消费
- 工作时间：默认 `08:30 - 18:00`，初版可只展示配置，不一定开放用户修改。

指标卡：

- 非工作时间 Token
- 总请求数
- 总消费
- 非工作时间 Token 占比

表格列：

- 排名
- 用户
- 请求数
- 非工作时间 Token
- 活跃时长
- 消费

不展示“主要非工作时段”列，价值不高且容易造成误解。

## 配置建议

```yaml
nonwork_usage:
  timezone: "Asia/Shanghai"
  work_start: "08:30"
  work_end: "18:00"
  active_gap_minutes: 5
  min_session_minutes: 1
  calendar:
    country: "CN"
    source: "holiday-cn"
    sync_enabled: true
    sync_schedule: "0 3 * * *"
    sync_years_ahead: 1
  aggregation:
    enabled: true
    schedule: "*/10 * * * *"
    recompute_days: 3
    retention_days: 3650
```

## 回填方案

上线步骤：

1. 建表：`calendar_days`、`calendar_sync_runs`、`usage_nonwork_daily_user_stats`。
2. 同步当前年份、上一年份、下一年份日历。
3. 对 `usage_logs` 保留窗口内数据做回填。
4. 如果已有更长期的账单/用量归档，可基于归档回填 Token 和请求，但活跃时长需要请求级时间点；没有请求级时间点时不能准确回填活跃时长。
5. 启用增量聚合任务。
6. 前端灰度展示。

历史数据限制：

- 如果原始请求日志已被清理，只剩日聚合数据，则无法还原 5 分钟会话化活跃时长。
- 可以保留 `active_duration_ms = null` 或标记 `duration_estimated=true`。
- 不建议用日请求数平均估算活跃时长，会误导用户。

## 边界条件清单

### 日历

- 自动来源某年不存在：生成预测日历，`confirmed=false`。
- 自动来源发布后变化：重导该年并触发聚合重算。
- 手工修正不能被自动同步覆盖。
- 调休上班日即使是周末，也必须算工作日。
- 普通周末必须入表，否则查询时需要临时推导，容易不一致。
- 节假日名称相同的调休上班日不要误判为休息日，必须以 `isOffDay` 为准。

### 时间

- 所有业务日历判断使用 `Asia/Shanghai`。
- 查询范围使用半开区间 `[start_date 00:00, end_date + 1 day 00:00)`。
- 不能用 `created_at::date`，因为数据库时区可能不是上海。
- 跨天会话必须拆分。
- 跨 08:30/18:00 会话必须拆分。

### 活跃时长

- 请求间隔等于 5 分钟是否计入要明确。建议 `<= 5 分钟` 计入。
- 单请求会话补 1 分钟，但不要对多请求会话的每个请求都补 1 分钟。
- 同一用户同一秒多个请求不应产生负数或重复时间。
- 并发请求只按请求开始时间序列计算，不按并发数叠加活跃时间。
- 重复 request_id 需要去重。
- 失败请求是否计入活跃时长要产品确认。建议只要进入 usage_logs 且属于用户主动请求就计入，系统内部探测请求不计入。

### 聚合

- 聚合重算要幂等。
- 增量聚合窗口要向前扩展 `active_gap_minutes`。
- 日历从预测变确认后要重算受影响年份或日期。
- 原始日志清理前必须先完成聚合。
- 聚合表保留时间应长于业务需要，建议至少 3 到 10 年。

### UI

- 不使用“异常时段”等带判断色彩的词。
- 活跃时长需要有 tooltip 或说明，避免被理解成 AI 回复耗时。
- 排名维度切换后，排名顺序和导出结果必须一致。
- 时间范围展示保持和现有 Token 排名页面一致。

## 测试计划

单元测试：

- 2026 年日历生成，验证节日休息日、调休上班日、普通周末。
- `isOffDay=false` 的调休上班日必须是工作日。
- 工作日 08:29 算下班时段，08:30 算工作时段，18:00 算下班时段。
- 非工作日全天算非工作日。
- 活跃时长 5 分钟边界。
- 单请求会话补 1 分钟。
- 跨 18:00 分摊。
- 跨 00:00 分摊。

集成测试：

- 同步任务从自动来源生成全年日历。
- 同步失败时保留预测日历。
- 手工覆盖不被同步覆盖。
- 聚合任务重跑同一日期结果一致。
- 原始日志清理后排名接口仍可查聚合历史。

前端测试：

- 时间范围控件与现有页面一致。
- 排名口径切换。
- 排名维度切换。
- 导出列与当前筛选一致。

## 推荐实施顺序

1. 先实现 `calendar_days` 和同步任务。
2. 再实现非工作时段判定函数和单元测试。
3. 实现活跃时长会话化算法。
4. 实现日聚合表和回填任务。
5. 新增排名接口。
6. 接入前端页面。
7. 加管理后台日历状态与手工修正能力。
