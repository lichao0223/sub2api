<template>
  <AppLayout>
    <div class="mx-auto max-w-[1600px] pb-24">
      <header class="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div>
          <p class="text-xs font-semibold uppercase tracking-[0.16em] text-primary-600 dark:text-primary-400">管理控制台</p>
          <h1 class="mt-1 text-2xl font-semibold tracking-tight text-gray-950 dark:text-white">AI 使用洞察</h1>
          <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-dark-300">通过脱敏采样了解团队如何使用 AI，结果基于抽样且可能不完整或误判。</p>
        </div>
        <div class="flex gap-2"><button type="button" class="btn btn-secondary" data-test="open-logs" :disabled="logsLoading" @click="openLogs">采样与分析日志</button><button type="button" class="btn btn-secondary" :disabled="loading" @click="refresh">刷新数据</button></div>
      </header>

      <div role="tablist" aria-label="AI 使用洞察" class="mb-4 tabs inline-flex">
        <button id="work-insight-tab" role="tab" type="button" class="tab" :class="{ 'tab-active': tab === 'insights' }" :aria-selected="tab === 'insights'" :tabindex="tab === 'insights' ? 0 : -1" aria-controls="work-insight-panel" @click="tab = 'insights'">洞察分析</button>
        <button id="work-config-tab" role="tab" type="button" class="tab" :class="{ 'tab-active': tab === 'config' }" :aria-selected="tab === 'config'" :tabindex="tab === 'config' ? 0 : -1" aria-controls="work-config-panel" @click="tab = 'config'">运行配置</button>
      </div>

      <div v-if="message" role="status" aria-live="polite" class="mb-4 rounded-lg border px-4 py-3 text-sm" :class="messageError ? 'border-red-200 bg-red-50 text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300' : 'border-green-200 bg-green-50 text-green-700 dark:border-green-900 dark:bg-green-950/30 dark:text-green-300'">{{ message }}</div>

      <section v-show="tab === 'insights'" id="work-insight-panel" role="tabpanel" aria-labelledby="work-insight-tab" class="space-y-5">
        <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
          <article v-for="metric in metrics" :key="metric.label" class="card p-4">
            <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ metric.label }}</p>
            <p class="mt-2 text-2xl font-semibold text-gray-950 dark:text-white">{{ metric.value }}</p>
            <p class="mt-1 text-xs text-gray-400">当前筛选结果</p>
          </article>
        </div>

        <section class="card p-4">
          <form class="flex flex-wrap items-end gap-3" @submit.prevent="search">
            <label class="field"><span>开始日期</span><input v-model="filters.start_date" type="date" class="input" /></label>
            <label class="field"><span>结束日期</span><input v-model="filters.end_date" type="date" class="input" /></label>
            <label class="field min-w-44 flex-1"><span>用户名</span><input v-model.trim="filters.user_name" class="input w-full" placeholder="用户名" /></label>
            <label class="field min-w-44 flex-1"><span>明确项目</span><input v-model.trim="filters.project_name" class="input w-full" placeholder="项目名称" /></label>
            <label class="field min-w-44"><span>任务类型</span><select v-model="filters.task_category" class="input"><option value="">全部类型</option><option v-for="category in TASK_CATEGORIES" :key="category">{{ category }}</option></select></label>
            <button type="submit" class="btn btn-primary">查询</button>
          </form>
        </section>

        <section class="card overflow-hidden">
          <div class="border-b border-gray-200 px-5 py-4 dark:border-dark-700">
            <h2 class="font-semibold text-gray-950 dark:text-white">用户洞察排名</h2>
            <p class="mt-1 text-xs text-gray-500">当前筛选时间范围内按 Token 用量排名，共 {{ page.total }} 位用户；点击查看最新一日详情。</p>
          </div>
          <DataTable :columns="columns" :data="page.items" :loading="loading" row-key="latest_insight_id" clickable-rows @row-click="openDetail">
            <template #cell-rank="{ row }"><strong>#{{ rankingPosition(row) }}</strong></template>
            <template #cell-user="{ row }"><div><strong>{{ row.username || `用户 ${row.user_id ?? '-'}` }}</strong><p class="text-xs text-gray-400">ID {{ row.user_id ?? '已删除' }}</p></div></template>
            <template #cell-usage="{ row }"><strong>{{ formatNumber(row.business_total_tokens) }}</strong><p class="text-xs text-gray-400">{{ row.business_request_count }} 次请求</p></template>
            <template #cell-sample_count="{ row }">{{ formatNumber(row.sample_count) }}</template>
            <template #cell-coverage="{ row }">{{ row.covered_active_session_count }} / {{ row.eligible_active_session_count }}</template>
            <template #cell-days="{ row }"><strong>{{ row.insight_days }}</strong><p class="text-xs text-gray-400">{{ formatRange(row.start_date, row.end_date) }}</p></template>
            <template #cell-summary="{ row }"><p class="max-w-md whitespace-pre-line text-sm">{{ row.latest_summary || '等待分析结果' }}</p></template>
            <template #cell-status="{ row }"><StatusBadge :status="row.failed_sample_count ? 'warning' : row.analyzed ? 'success' : 'inactive'" :label="row.failed_sample_count ? '部分失败' : row.analyzed ? '已完成' : '分析中'" /></template>
          </DataTable>
          <Pagination :total="page.total" :page="page.page" :page-size="page.page_size" @update:page="changePage" @update:page-size="changePageSize" />
        </section>
      </section>

      <section v-if="draft" v-show="tab === 'config'" id="work-config-panel" role="tabpanel" aria-labelledby="work-config-tab" class="grid gap-5 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div class="space-y-5">
          <ConfigCard title="采样开关" description="控制新的业务请求是否进入异步采样队列">
            <label class="flex items-center justify-between gap-4"><span><strong>启用采样分析</strong><small>关闭后历史洞察仍可查看</small></span><Toggle v-model="draft.enabled" aria-label="启用采样分析" /></label>
            <div data-test="sample-rate-config" class="mt-5 border-t border-gray-200 pt-5 dark:border-dark-700">
              <NumberField v-model="draft.sample_rate" label="请求采样率" suffix="%" :min="0" :max="100" />
              <p class="mt-2 text-xs text-gray-500">每个新会话的首个请求必定采样，后续请求按此比例采样；默认 20%。</p>
            </div>
          </ConfigCard>

          <ConfigCard title="采样限制与排除" description="按用户活跃会话采样，同一用户的多个 API Key 共享状态">
            <div class="form-grid">
              <NumberField v-model="draft.session_idle_minutes" label="会话空闲阈值" suffix="分钟" :min="1" />
              <NumberField v-model="draft.user_daily_limit" label="单用户每日上限" suffix="次" :min="1" :max="100000" />
              <NumberField v-model="draft.global_daily_limit" label="全局每日上限" suffix="次" :min="1" :max="500000" />
              <label class="field"><span>统计时区</span><select v-model="draft.timezone" class="input"><option>Asia/Shanghai</option><option>UTC</option><option>Asia/Tokyo</option></select></label>
              <label class="field"><span>排除用户 ID</span><input :value="draft.excluded_user_ids.join(',')" class="input" placeholder="1,2,3" @input="draft.excluded_user_ids = parseIDs(($event.target as HTMLInputElement).value)" /></label>
              <label class="field sm:col-span-2"><span>排除用户邮箱</span><input :value="draft.excluded_user_emails.join(',')" class="input" placeholder="user@example.com" @input="draft.excluded_user_emails = splitList(($event.target as HTMLInputElement).value)" /></label>
            </div>
          </ConfigCard>

          <ConfigCard title="分析器连接" description="仅支持 OpenAI 兼容接口；账号管理模式只显示 OpenAI 平台账号">
            <div class="mb-4 flex gap-2" role="radiogroup" aria-label="分析器来源">
              <button type="button" role="radio" :aria-checked="draft.analyzer_source === 'account'" class="btn" :class="draft.analyzer_source === 'account' ? 'btn-primary' : 'btn-secondary'" @click="draft.analyzer_source = 'account'">账号管理</button>
              <button type="button" role="radio" :aria-checked="draft.analyzer_source === 'custom'" class="btn" :class="draft.analyzer_source === 'custom' ? 'btn-primary' : 'btn-secondary'" @click="draft.analyzer_source = 'custom'">自定义节点</button>
            </div>
            <div class="form-grid">
              <label v-if="draft.analyzer_source === 'account'" class="field"><span>OpenAI 平台分析账号</span><select v-model.number="draft.analyzer_account_id" class="input"><option :value="0" disabled>请选择 OpenAI 平台账号</option><option v-for="account in analyzerAccounts" :key="account.id" :value="account.id">{{ account.name }}</option></select><small v-if="!analyzerAccounts.length">账号管理中暂无可用的 OpenAI 平台账号</small></label>
              <label v-else class="field sm:col-span-2"><span>OpenAI 兼容 API Endpoint</span><input v-model.trim="draft.analyzer_base_url" class="input" placeholder="https://example.com/v1" /></label>
              <label v-if="draft.analyzer_source === 'custom'" class="field"><span>API Key</span><input v-model.trim="draft.analyzer_token" type="password" class="input" :placeholder="draft.analyzer_token_set ? '已配置，留空保持不变' : '输入 API Key'" autocomplete="new-password" /></label>
              <label class="field"><span>分析模型</span><select v-if="selectedAccount?.models.length" v-model="draft.analyzer_model" class="input"><option v-for="model in selectedAccount.models" :key="model">{{ model }}</option></select><input v-else v-model.trim="draft.analyzer_model" class="input" placeholder="qwen3.6-35b" /></label>
              <button type="button" class="btn btn-secondary self-end" :disabled="probing" @click="probeAnalyzer">{{ probing ? '检测中…' : '测试连接' }}</button>
            </div>
          </ConfigCard>

          <ConfigCard title="分析调度与上下文" description="按用户归桶；原始样本只分析一次，历史只合并结构化摘要">
            <div class="mb-4 flex items-center justify-between gap-4 rounded-lg bg-gray-50 p-3 dark:bg-dark-800"><span class="text-sm"><strong>手动触发</strong><small>将当前待分析样本立即归入批次，由后台异步处理</small></span><button type="button" class="btn btn-primary" data-test="analyze-now" :disabled="analyzing || dirty || !draft.enabled" @click="analyzeNow">{{ analyzing ? '正在创建批次…' : '立即分析' }}</button></div>
            <div class="form-grid">
              <label class="field"><span>触发策略</span><select v-model="draft.analysis_trigger_mode" class="input"><option value="hybrid">混合触发（推荐）</option><option value="fixed_interval">固定间隔</option><option value="fixed_time">固定时点</option></select></label>
              <NumberField v-if="draft.analysis_trigger_mode === 'hybrid'" v-model="draft.analysis_idle_minutes" label="用户空闲触发" suffix="分钟" :min="1" />
              <NumberField v-if="draft.analysis_trigger_mode === 'hybrid'" v-model="draft.analysis_max_wait_minutes" label="批次最长等待" suffix="分钟" :min="1" />
              <NumberField v-if="draft.analysis_trigger_mode === 'fixed_interval'" v-model="draft.analysis_fixed_interval_minutes" label="分析间隔" suffix="分钟" :min="1" />
              <label v-if="draft.analysis_trigger_mode === 'fixed_time'" class="field"><span>执行时点</span><input :value="draft.analysis_fixed_times.join(',')" class="input" placeholder="09:00,18:00" @input="draft.analysis_fixed_times = splitList(($event.target as HTMLInputElement).value)" /></label>
              <NumberField v-model="draft.max_samples_per_batch" label="单批最大样本" suffix="条" :min="1" />
              <label class="field"><span>每日收口时间</span><input v-model="draft.daily_finalize_time" type="time" class="input" /></label>
              <NumberField v-model="draft.context_window_tokens" label="模型上下文" suffix="Token" :min="8000" />
              <NumberField v-model="draft.max_input_tokens" label="单次最大输入" suffix="Token" :min="1000" />
              <NumberField v-model="draft.reserved_output_tokens" label="输出预留" suffix="Token" :min="1000" />
              <NumberField v-model="draft.analysis_timeout_seconds" label="分析超时" suffix="秒" :min="1" />
            </div>
            <p class="mt-4 rounded-lg px-3 py-2 text-xs" :class="budgetSafe ? 'bg-green-50 text-green-700 dark:bg-green-950/30 dark:text-green-300' : 'bg-red-50 text-red-700 dark:bg-red-950/30 dark:text-red-300'">{{ budgetSafe ? '上下文预算安全' : `单次输入超过安全上限 ${formatNumber(safeInputBudget)} Token` }}</p>
          </ConfigCard>

          <ConfigCard title="数据生命周期" description="原始文本只在 Redis 短期存在；样本批次与每日洞察分开保留">
            <label class="mb-4 flex items-center justify-between gap-4"><span><strong>启用定时清理</strong><small>关闭后数据将持续增长</small></span><Toggle v-model="draft.cleanup_enabled" aria-label="启用定时清理" /></label>
            <label class="mb-4 flex items-center justify-between gap-4"><span><strong>保存脱敏预览</strong><small>默认关闭；开启后仅保存短且不可还原的脱敏片段</small></span><Toggle v-model="draft.store_redacted_preview" aria-label="保存脱敏预览" /></label>
            <div class="form-grid">
              <NumberField v-model="draft.max_job_age_minutes" label="任务最大年龄" suffix="分钟" :min="1" />
              <NumberField v-model="draft.payload_ttl_minutes" label="Redis 文本 TTL" suffix="分钟" :min="1" />
              <NumberField v-model="draft.sample_retention_days" label="样本/批次保留" suffix="天" :min="1" />
              <NumberField v-model="draft.insight_retention_days" label="每日洞察保留" suffix="天" :min="1" />
              <label class="field"><span>清理时间</span><input v-model="draft.cleanup_time" type="time" class="input" /></label>
              <NumberField v-model="draft.cleanup_batch_size" label="单批清理上限" suffix="条" :min="100" />
            </div>
          </ConfigCard>

          <ConfigCard title="任务队列" description="设置后台并发与积压保护">
            <div class="form-grid"><NumberField v-model="draft.worker_count" label="Worker 数量" suffix="个" :min="1" /><NumberField v-model="draft.queue_capacity" label="最大队列长度" suffix="条" :min="100" /></div>
          </ConfigCard>
        </div>

        <aside class="card h-fit p-5 xl:sticky xl:top-5">
          <h2 class="font-semibold text-gray-950 dark:text-white">运行状态</h2>
          <dl class="mt-4 space-y-4 text-sm">
            <div><dt>当前采样策略</dt><dd>首请求必采 · 后续 {{ draft.sample_rate }}%</dd></div>
            <div><dt>本地队列</dt><dd>{{ runtime?.queue_depth ?? 0 }} / {{ runtime?.queue_capacity ?? draft.queue_capacity }} 条 · {{ formatBytes(runtime?.queue_bytes ?? 0) }} / {{ formatBytes(runtime?.queue_byte_capacity ?? 0) }}</dd></div>
            <div><dt>已处理候选</dt><dd>{{ formatNumber(runtime?.processed ?? 0) }}</dd></div>
            <div><dt>忙丢弃</dt><dd>{{ formatNumber(runtime?.dropped ?? 0) }}</dd></div>
            <div><dt>处理失败</dt><dd :class="runtime?.failed ? 'text-red-600' : ''">{{ formatNumber(runtime?.failed ?? 0) }}</dd></div>
            <div><dt>待归桶 / 排队 / 重试</dt><dd>{{ formatNumber(runtime?.waiting_samples ?? 0) }} / {{ formatNumber(runtime?.queued_batches ?? 0) }} / {{ formatNumber(runtime?.retry_batches ?? 0) }}</dd></div>
            <div><dt>处理中 / 今日完成</dt><dd>{{ formatNumber(runtime?.processing_batches ?? 0) }} / {{ formatNumber(runtime?.done_batches ?? 0) }}</dd></div>
            <div><dt>今日会话覆盖</dt><dd>{{ formatNumber(runtime?.covered_sessions ?? 0) }} / {{ formatNumber(runtime?.active_sessions ?? 0) }}</dd></div>
            <div><dt>今日分析 Token</dt><dd>{{ formatNumber((runtime?.analyzer_input_tokens ?? 0) + (runtime?.analyzer_output_tokens ?? 0)) }}</dd></div>
            <div><dt>数据保留</dt><dd>{{ draft.sample_retention_days }} / {{ draft.insight_retention_days }} 天</dd></div>
          </dl>
          <p class="mt-5 rounded-lg bg-primary-50 p-3 text-xs text-primary-800 dark:bg-primary-950/30 dark:text-primary-200">原始样本仅用于一次增量分析，管理端不保存或展示原始对话。</p>
        </aside>
      </section>
    </div>

    <div v-if="draft && tab === 'config'" class="fixed inset-x-0 bottom-0 z-30 border-t border-gray-200 bg-white/95 px-4 py-3 shadow-lg backdrop-blur dark:border-dark-700 dark:bg-dark-900/95 lg:left-64">
      <div class="mx-auto flex max-w-[1600px] items-center justify-between gap-3"><span class="text-sm text-gray-500">{{ dirty ? '有未保存的更改' : '所有更改均已保存' }}</span><div class="flex gap-2"><button type="button" class="btn btn-secondary" :disabled="!dirty || saving" @click="resetConfig">撤销更改</button><button type="button" class="btn btn-primary" :disabled="!dirty || saving || !budgetSafe" @click="saveConfig">{{ saving ? '保存中…' : '保存配置' }}</button></div></div>
    </div>

    <BaseDialog :show="detailOpen" :title="detail ? `${detail.insight.username} 的 AI 工作洞察` : 'AI 工作洞察'" width="wide" close-on-click-outside @close="detailOpen = false">
      <div v-if="detail" class="space-y-5">
        <p class="text-xs text-gray-500">{{ formatDate(detail.insight.insight_date) }} · 最后分析 {{ formatDateTime(detail.insight.last_analyzed_at || '') }} · 基于抽样，可能不完整或误判</p>
        <section class="rounded-xl bg-primary-50 p-4 dark:bg-primary-950/30"><h3 class="text-xs font-semibold text-primary-700 dark:text-primary-300">最新工作摘要</h3><p class="mt-2 whitespace-pre-line text-sm leading-6 text-gray-800 dark:text-dark-100">{{ detail.insight.daily_summary || '暂无摘要' }}</p></section>
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-4"><article class="mini-stat"><span>成功请求</span><strong>{{ formatNumber(detail.insight.business_request_count) }}</strong></article><article class="mini-stat"><span>请求成功率</span><strong>{{ successRate(detail.insight) }}</strong></article><article class="mini-stat"><span>平均耗时</span><strong>{{ formatDuration(detail.insight.average_duration_ms) }}</strong></article><article class="mini-stat"><span>P95 耗时</span><strong>{{ formatDuration(detail.insight.p95_duration_ms) }}</strong></article></div>
        <section><h3 class="section-title">Token 构成</h3><div class="mt-2 grid grid-cols-2 gap-3 sm:grid-cols-3"><article class="mini-stat"><span>输入</span><strong>{{ formatNumber(detail.insight.business_input_tokens) }}</strong></article><article class="mini-stat"><span>输出</span><strong>{{ formatNumber(detail.insight.business_output_tokens) }}</strong></article><article class="mini-stat"><span>缓存读取</span><strong>{{ formatNumber(detail.insight.business_cache_read_tokens) }}</strong></article></div></section>
        <section><h3 class="section-title">使用模型</h3><div class="mt-2 flex flex-wrap gap-2"><span v-for="([model, count]) in Object.entries(detail.insight.model_usage)" :key="model" class="badge">{{ model }} · {{ count }} 次</span><span v-if="!Object.keys(detail.insight.model_usage).length" class="text-sm text-gray-400">暂无模型统计</span></div></section>
        <section><h3 class="section-title">任务分类</h3><div class="mt-2 flex flex-wrap gap-2"><span v-for="([category, count]) in Object.entries(detail.insight.task_category_stats).sort((a, b) => b[1] - a[1])" :key="category" class="badge">{{ category }} · {{ count }}</span><span v-if="!Object.keys(detail.insight.task_category_stats).length" class="text-sm text-gray-400">暂无任务分类</span></div></section>
        <section><h3 class="section-title">明确项目与模块</h3><div class="mt-2 flex flex-wrap gap-2"><span v-for="item in [...detail.insight.explicit_projects, ...detail.insight.explicit_modules]" :key="item" class="badge">{{ item }}</span><span v-if="!detail.insight.explicit_projects.length && !detail.insight.explicit_modules.length" class="text-sm text-gray-400">输入中未出现明确项目或模块</span></div></section>
        <section v-if="detail.insight.change_types.length || detail.insight.business_topics.length"><h3 class="section-title">变更类型与业务主题</h3><div class="mt-2 flex flex-wrap gap-2"><span v-for="item in [...detail.insight.change_types, ...detail.insight.business_topics]" :key="item" class="badge">{{ item }}</span></div></section>
        <section><div class="flex items-center justify-between"><h3 class="section-title">代表性工作样本</h3><span class="text-xs text-gray-500">共 {{ detail.representative_item_count }} 条</span></div><div class="mt-3 space-y-2"><article v-for="(item, index) in visibleSamples" :key="`${item.summary}-${index}`" class="flex gap-3 rounded-lg border border-gray-200 p-3 dark:border-dark-700"><span class="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary-50 text-xs font-semibold text-primary-700 dark:bg-primary-950 dark:text-primary-300">{{ index + 1 }}</span><div><strong class="text-sm leading-6">{{ item.summary }}</strong><p class="mt-1 text-xs text-gray-500">{{ [...item.task_categories, ...item.explicit_projects, ...item.explicit_modules].join(' · ') }}</p></div></article><p v-if="detail.representative_items_expired" class="text-sm text-amber-600">样本详情已超过保留期，摘要仍可查看。</p></div><button v-if="detail.representative_items.length > 4" type="button" class="btn btn-secondary btn-sm mt-3" @click="showAllSamples = !showAllSamples">{{ showAllSamples ? '收起样本' : `展开其余 ${detail.representative_items.length - 4} 条` }}</button></section>
        <p v-if="detail.insight.business_error_count || detail.insight.failed_sample_count" class="rounded-lg bg-amber-50 p-3 text-xs text-amber-700 dark:bg-amber-950/30 dark:text-amber-300">当日有 {{ detail.insight.business_error_count }} 个业务错误、{{ detail.insight.failed_sample_count }} 个洞察样本失败，结果可能不完整。</p>
        <p class="rounded-lg bg-gray-50 p-3 text-xs text-gray-500 dark:bg-dark-800">隐私保护：只展示聚合分类与脱敏摘要，不展示 Redis 临时文本、原始提示词或模型原始响应。</p>
      </div>
      <p v-else class="py-8 text-center text-sm text-gray-500">正在加载详情…</p>
    </BaseDialog>

    <BaseDialog :show="logsOpen" title="采样与分析日志" width="wide" close-on-click-outside @close="closeLogs">
      <div class="space-y-6">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <p class="text-xs text-gray-500">展示数据库中保留的全部历史元数据并分页，不展示原始提示词、模型输出或 Redis 临时文本。</p>
          <button type="button" class="btn btn-danger btn-sm" data-test="clear-logs" @click="clearLogsConfirm = true">清空历史日志</button>
        </div>
        <div class="grid gap-2 text-xs sm:grid-cols-4">
          <div class="log-stat"><span>排队等待</span><strong>{{ runtime?.queued_batches ?? 0 }}</strong></div>
          <div class="log-stat"><span>正在分析</span><strong>{{ runtime?.processing_batches ?? 0 }}</strong></div>
          <div class="log-stat"><span>等待重试</span><strong>{{ runtime?.retry_batches ?? 0 }}</strong></div>
          <div class="log-stat"><span>分析失败</span><strong>{{ runtime?.failed_batches ?? 0 }}</strong></div>
        </div>

        <section class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
          <div class="bg-gray-50 px-4 py-3 dark:bg-dark-800"><h3 class="section-title">分析任务</h3><p class="mt-1 text-xs text-gray-500">停止正在执行的任务后会回到待分析，可按需重新开始。</p></div>
          <div role="tablist" aria-label="分析任务状态" class="tabs m-3 inline-flex">
            <button type="button" role="tab" class="tab" :class="{ 'tab-active': batchLogTab === 'pending' }" :aria-selected="batchLogTab === 'pending'" @click="batchLogTab = 'pending'">待分析 {{ pendingLogPage.total }}</button>
            <button type="button" role="tab" class="tab" :class="{ 'tab-active': batchLogTab === 'processing' }" :aria-selected="batchLogTab === 'processing'" @click="batchLogTab = 'processing'">正在分析 {{ processingLogPage.total }}</button>
            <button type="button" role="tab" class="tab" :class="{ 'tab-active': batchLogTab === 'errors' }" :aria-selected="batchLogTab === 'errors'" @click="batchLogTab = 'errors'">错误 {{ errorLogPage.total }}</button>
          </div>

          <div v-show="batchLogTab === 'pending'" role="tabpanel">
            <div class="overflow-x-auto"><table class="w-full text-left text-xs"><thead><tr class="border-b dark:border-dark-700"><th class="p-3">时间</th><th class="p-3">用户</th><th class="p-3">样本</th><th class="p-3">触发原因</th><th class="p-3">状态</th><th class="p-3 text-right">操作</th></tr></thead><tbody>
              <tr v-for="item in pendingLogPage.items" :key="item.id" class="border-b last:border-0 dark:border-dark-800"><td class="p-3 whitespace-nowrap">{{ formatDateTime(item.created_at) }}</td><td class="p-3">{{ logUserLabel(item.username, item.user_id) }}</td><td class="p-3">{{ item.sample_count }}</td><td class="p-3">{{ triggerReasonLabel(item.trigger_reason) }}</td><td class="p-3">{{ item.error_code === 'admin_stopped' ? '已暂停，待重新分析' : statusLabel(item.status) }}</td><td class="p-3 text-right"><button v-if="item.error_code === 'admin_stopped'" type="button" class="btn btn-primary btn-xs" :disabled="retryingBatchID !== null" @click="retryBatch(item)">{{ retryingBatchID === item.id ? '开始中…' : '开始分析' }}</button><button v-else type="button" class="btn btn-secondary btn-xs" :disabled="stoppingBatchID !== null" @click="stopBatch(item)">{{ stoppingBatchID === item.id ? '暂停中…' : '暂停排队' }}</button></td></tr>
              <tr v-if="!logsLoading && !pendingLogPage.items.length"><td colspan="6" class="p-8 text-center text-gray-400">暂无待分析任务</td></tr>
            </tbody></table></div>
            <Pagination :total="pendingLogPage.total" :page="pendingLogPage.page" :page-size="pendingLogPage.page_size" @update:page="value => changeLogPage(pendingLogPage, value, loadPendingLogs)" @update:page-size="value => changeLogPageSize(pendingLogPage, value, loadPendingLogs)" />
          </div>

          <div v-show="batchLogTab === 'processing'" role="tabpanel">
            <div class="overflow-x-auto"><table class="w-full text-left text-xs"><thead><tr class="border-b dark:border-dark-700"><th class="p-3">时间</th><th class="p-3">用户</th><th class="p-3">样本</th><th class="p-3">触发原因</th><th class="p-3">状态</th><th class="p-3 text-right">操作</th></tr></thead><tbody>
              <tr v-for="item in processingLogPage.items" :key="item.id" class="border-b last:border-0 dark:border-dark-800"><td class="p-3 whitespace-nowrap">{{ formatDateTime(item.created_at) }}</td><td class="p-3">{{ logUserLabel(item.username, item.user_id) }}</td><td class="p-3">{{ item.sample_count }}</td><td class="p-3">{{ triggerReasonLabel(item.trigger_reason) }}</td><td class="p-3">正在分析</td><td class="p-3 text-right"><button type="button" class="btn btn-danger btn-xs" :disabled="stoppingBatchID !== null" @click="stopBatch(item)">{{ stoppingBatchID === item.id ? '停止中…' : '停止' }}</button></td></tr>
              <tr v-if="!logsLoading && !processingLogPage.items.length"><td colspan="6" class="p-8 text-center text-gray-400">当前没有正在分析的任务</td></tr>
            </tbody></table></div>
            <Pagination :total="processingLogPage.total" :page="processingLogPage.page" :page-size="processingLogPage.page_size" @update:page="value => changeLogPage(processingLogPage, value, loadProcessingLogs)" @update:page-size="value => changeLogPageSize(processingLogPage, value, loadProcessingLogs)" />
          </div>

          <div v-show="batchLogTab === 'errors'" role="tabpanel">
            <div class="flex items-center justify-between border-y border-red-100 bg-red-50 px-4 py-3 dark:border-red-900/50 dark:bg-red-950/20"><p class="text-xs text-red-600 dark:text-red-400">失败任务可逐条或全部重新分析。</p><button type="button" class="btn btn-primary btn-sm" data-test="retry-all-errors" :disabled="retryingAll || !errorLogPage.total" @click="retryAllBatches">{{ retryingAll ? '正在重新排队…' : '全部重新分析' }}</button></div>
            <div class="overflow-x-auto"><table class="w-full text-left text-xs"><thead><tr class="border-b border-red-100 dark:border-red-900/50"><th class="p-3">时间</th><th class="p-3">用户</th><th class="p-3">样本</th><th class="p-3">错误原因</th><th class="p-3">状态</th><th class="p-3 text-right">操作</th></tr></thead><tbody>
              <tr v-for="item in errorLogPage.items" :key="item.id" class="border-b border-red-100 last:border-0 dark:border-red-950"><td class="p-3 whitespace-nowrap">{{ formatDateTime(item.created_at) }}</td><td class="p-3">{{ logUserLabel(item.username, item.user_id) }}</td><td class="p-3">{{ item.sample_count }}</td><td class="p-3 text-red-600 dark:text-red-400">{{ errorLabel(item.error_code) }}</td><td class="p-3">{{ statusLabel(item.status) }}</td><td class="p-3"><div class="flex justify-end gap-2"><button type="button" class="btn btn-primary btn-xs" :disabled="retryingBatchID !== null || retryingAll" @click="retryBatch(item)">{{ retryingBatchID === item.id ? '重新排队中…' : '重新分析' }}</button><button v-if="item.status === 'retry'" type="button" class="btn btn-secondary btn-xs" :disabled="stoppingBatchID !== null" @click="stopBatch(item)">{{ stoppingBatchID === item.id ? '暂停中…' : '暂停重试' }}</button></div></td></tr>
              <tr v-if="!logsLoading && !errorLogPage.items.length"><td colspan="6" class="p-8 text-center text-gray-400">暂无错误任务</td></tr>
            </tbody></table></div>
            <Pagination :total="errorLogPage.total" :page="errorLogPage.page" :page-size="errorLogPage.page_size" @update:page="value => changeLogPage(errorLogPage, value, loadErrorLogs)" @update:page-size="value => changeLogPageSize(errorLogPage, value, loadErrorLogs)" />
          </div>
        </section>

        <section class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
          <div class="bg-gray-50 px-4 py-3 dark:bg-dark-800"><h3 class="section-title">已完成记录</h3><p class="mt-1 text-xs text-gray-500">共 {{ completedLogPage.total }} 条历史记录。</p></div>
          <div class="overflow-x-auto"><table class="w-full text-left text-xs"><thead><tr class="border-b dark:border-dark-700"><th class="p-3">时间</th><th class="p-3">用户</th><th class="p-3">样本</th><th class="p-3">触发原因</th><th class="p-3">模型</th><th class="p-3">分析 Token</th><th class="p-3">状态</th></tr></thead><tbody>
            <tr v-for="item in completedLogPage.items" :key="item.id" class="border-b last:border-0 dark:border-dark-800"><td class="p-3 whitespace-nowrap">{{ formatDateTime(item.created_at) }}</td><td class="p-3">{{ logUserLabel(item.username, item.user_id) }}</td><td class="p-3">{{ item.sample_count }}</td><td class="p-3">{{ triggerReasonLabel(item.trigger_reason) }}</td><td class="p-3">{{ item.analyzer_model || '—' }}</td><td class="p-3">{{ formatNumber(item.analyzer_input_tokens + item.analyzer_output_tokens) }}</td><td class="p-3">分析完成</td></tr>
            <tr v-if="!logsLoading && !completedLogPage.items.length"><td colspan="7" class="p-8 text-center text-gray-400">暂无已完成记录</td></tr>
          </tbody></table></div>
          <Pagination :total="completedLogPage.total" :page="completedLogPage.page" :page-size="completedLogPage.page_size" @update:page="value => changeLogPage(completedLogPage, value, loadCompletedLogs)" @update:page-size="value => changeLogPageSize(completedLogPage, value, loadCompletedLogs)" />
        </section>

        <section class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
          <div class="bg-gray-50 px-4 py-3 dark:bg-dark-800"><h3 class="section-title">采样日志</h3><p class="mt-1 text-xs text-gray-500">共 {{ sampleLogPage.total }} 条。字符数为“脱敏分析文本 / 用户输入”，相同字符数不代表输入内容相同。</p></div>
          <div class="overflow-x-auto"><table class="w-full text-left text-xs"><thead><tr class="border-b dark:border-dark-700"><th class="p-3">时间</th><th class="p-3">用户</th><th class="p-3">模型</th><th class="p-3">采样原因</th><th class="p-3">分析文本估算</th><th class="p-3">状态 / 异常</th></tr></thead><tbody>
            <tr v-for="item in sampleLogPage.items" :key="item.id" class="border-b last:border-0 dark:border-dark-800"><td class="p-3 whitespace-nowrap">{{ formatDateTime(item.created_at) }}</td><td class="p-3">{{ logUserLabel(item.username, item.user_id) }}</td><td class="p-3">{{ item.requested_model || item.provider }}</td><td class="p-3">{{ sampleReasonLabel(item.sample_reason) }}</td><td class="p-3 whitespace-nowrap">≈ {{ formatNumber(item.estimated_tokens) }} Token<p class="mt-1 text-gray-400">脱敏后 {{ formatNumber(item.analyzed_chars) }} / 原文 {{ formatNumber(item.prompt_chars) }} 字符<span v-if="item.truncated" class="text-amber-600"> · 历史截断记录</span></p></td><td class="p-3"><span>{{ statusLabel(item.status) }}</span><p v-if="item.error_code" class="mt-1 text-red-600">{{ sampleErrorLabel(item.error_code) }}</p></td></tr>
            <tr v-if="!logsLoading && !sampleLogPage.items.length"><td colspan="6" class="p-8 text-center text-gray-400">暂无采样记录</td></tr>
          </tbody></table></div>
          <Pagination :total="sampleLogPage.total" :page="sampleLogPage.page" :page-size="sampleLogPage.page_size" @update:page="value => changeLogPage(sampleLogPage, value, loadSampleLogs)" @update:page-size="value => changeLogPageSize(sampleLogPage, value, loadSampleLogs)" />
        </section>
        <p v-if="logsLoading" class="py-4 text-center text-sm text-gray-500">正在加载日志…</p>
      </div>
    </BaseDialog>
    <ConfirmDialog :show="clearLogsConfirm" title="清空历史日志" message="确认清空已完成和失败的采样、分析日志？待分析、正在分析和每日洞察结果会保留，此操作不可恢复。" confirm-text="确认清空" :loading="clearingLogs" danger @confirm="clearLogs" @cancel="clearLogsConfirm = false" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, onUnmounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import Toggle from '@/components/common/Toggle.vue'
import type { Column } from '@/components/common/types'
import api from './api'
import { TASK_CATEGORIES } from './types'
import type { AnalyzerAccount, BatchSummary, DailyInsight, DailyInsightDetail, DailyInsightFilters, LogPage, SampleSummary, UserInsightRanking, UserInsightRankingPage, WorkInsightConfig, WorkInsightOverview, WorkInsightRuntime } from './types'

const ConfigCard = defineComponent({ props: { title: { type: String, required: true }, description: { type: String, required: true } }, setup: (props, { slots }) => () => h('section', { class: 'card p-5' }, [h('div', { class: 'mb-5' }, [h('h2', { class: 'font-semibold text-gray-950 dark:text-white' }, props.title), h('p', { class: 'mt-1 text-xs text-gray-500' }, props.description)]), slots.default?.()]) })
const NumberField = defineComponent({ props: { modelValue: { type: Number, default: 0 }, label: { type: String, required: true }, suffix: { type: String, default: '' }, min: { type: Number, default: 0 }, max: { type: Number, default: undefined } }, emits: ['update:modelValue'], setup: (props, { emit }) => () => h('label', { class: 'field' }, [h('span', props.label), h('div', { class: 'relative' }, [h('input', { type: 'number', min: props.min, max: props.max, value: props.modelValue ?? 0, class: 'input w-full pr-16', onInput: (event: Event) => emit('update:modelValue', Number((event.target as HTMLInputElement).value)) }), props.suffix && h('em', { class: 'pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs not-italic text-gray-400' }, props.suffix)])]) })

const tab = ref<'insights' | 'config'>('insights')
const loading = ref(false)
const saving = ref(false)
const probing = ref(false)
const analyzing = ref(false)
const logsLoading = ref(false)
const clearLogsConfirm = ref(false)
const clearingLogs = ref(false)
const retryingBatchID = ref<number | null>(null)
const stoppingBatchID = ref<number | null>(null)
const retryingAll = ref(false)
const message = ref('')
const messageError = ref(false)
const runtime = ref<WorkInsightRuntime | null>(null)
const overview = ref<WorkInsightOverview | null>(null)
const analyzerAccounts = ref<AnalyzerAccount[]>([])
const savedConfig = ref<WorkInsightConfig | null>(null)
const draft = ref<WorkInsightConfig | null>(null)
const page = reactive<UserInsightRankingPage>({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
const filters = reactive<DailyInsightFilters>({ start_date: '', end_date: '', user_name: '', task_category: '', project_name: '' })
const appliedFilters = ref<DailyInsightFilters>({ ...filters })
const detailOpen = ref(false)
const detail = ref<DailyInsightDetail | null>(null)
const showAllSamples = ref(false)
const logsOpen = ref(false)
const batchLogTab = ref<'pending' | 'processing' | 'errors'>('pending')
const sampleLogPage = reactive<LogPage<SampleSummary>>({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
const pendingLogPage = reactive<LogPage<BatchSummary>>({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
const processingLogPage = reactive<LogPage<BatchSummary>>({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
const completedLogPage = reactive<LogPage<BatchSummary>>({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
const errorLogPage = reactive<LogPage<BatchSummary>>({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
let logRefreshTimer: ReturnType<typeof setTimeout> | undefined
const columns: Column[] = [
  { key: 'rank', label: '排名' }, { key: 'user', label: '用户' }, { key: 'usage', label: 'Token 用量' },
  { key: 'sample_count', label: '采样数' }, { key: 'coverage', label: '会话覆盖' }, { key: 'days', label: '洞察天数' },
  { key: 'summary', label: '最新工作摘要' }, { key: 'status', label: '状态' },
]
const metrics = computed(() => {
  const eligible = overview.value?.active_sessions ?? 0
  const covered = overview.value?.covered_sessions ?? 0
  return [
    { label: '有洞察结果用户', value: formatNumber(overview.value?.insight_users ?? 0) },
    { label: '活跃会话', value: formatNumber(eligible) },
    { label: '已覆盖会话', value: formatNumber(covered) },
    { label: '会话覆盖率', value: eligible ? `${(covered / eligible * 100).toFixed(1)}%` : '—' },
    { label: '采样请求', value: formatNumber(overview.value?.sample_requests ?? 0) },
  ]
})
const dirty = computed(() => JSON.stringify(draft.value) !== JSON.stringify(savedConfig.value))
const safeInputBudget = computed(() => draft.value ? draft.value.context_window_tokens - draft.value.reserved_output_tokens - 4000 - Math.floor(draft.value.context_window_tokens * 0.15) : 0)
const budgetSafe = computed(() => !!draft.value && draft.value.max_input_tokens <= safeInputBudget.value)
const visibleSamples = computed(() => showAllSamples.value ? detail.value?.representative_items ?? [] : detail.value?.representative_items.slice(0, 4) ?? [])
const selectedAccount = computed(() => analyzerAccounts.value.find(account => account.id === draft.value?.analyzer_account_id))

const sampleReasonLabels: Record<string, string> = { session_coverage: '新会话首请求必采', rate: '采样率命中', compact: '会话压缩快照' }
const triggerReasonLabels: Record<string, string> = { idle: '用户空闲触发', max_wait: '达到最长等待时间', sample_limit: '达到单批样本上限', token_limit: '达到单批 Token 上限', fixed: '定时策略触发', manual: '管理员手动触发', finalize: '每日收口触发' }
const statusLabels: Record<string, string> = { staging: '正在入库', pending_batch: '等待组成分析批次', batched: '已进入分析批次', queued: '排队等待分析', processing: '正在分析', retry: '异常，等待自动重试', analyzed: '分析完成', done: '分析完成', failed: '分析失败', dropped: '已停止处理' }
const errorLabels: Record<string, string> = {
  summary_write_conflict: '摘要写入版本冲突，自动重试后仍未成功', summary_json_invalid: '分析结果包含无效的 JSON 数组', summary_write_failed: '摘要写入数据库失败，请查看后台错误日志', summary_load_failed: '读取已有摘要失败',
  sample_load_failed: '读取样本记录失败', samples_missing: '分析批次没有可用样本', payload_missing: 'Redis 中的临时请求文本已不存在', payload_empty: '请求文本为空，无法分析',
  analyzer_config_invalid: '分析模型配置无效', analyzer_context_length: '输入超过分析模型上下文限制', analyzer_invalid_result: '分析模型返回的结果格式无效', analyzer_unavailable: '分析模型暂时不可用',
  job_expired: '任务超过最大等待时间', user_deleted: '用户已删除，停止分析', admin_stopped: '管理员手动停止', payload_capacity: 'Redis 临时文本容量已满', payload_store_failed: '请求文本写入 Redis 失败', payload_encode_failed: '请求文本编码失败', payload_too_large: '请求文本超过大小限制', queue_publish_failed: '采样任务加入队列失败',
}

function clone<T>(value: T): T { return JSON.parse(JSON.stringify(value)) as T }
function splitList(value: string): string[] { return [...new Set(value.split(',').map(item => item.trim()).filter(Boolean))] }
function parseIDs(value: string): number[] { return splitList(value).map(Number).filter(value => Number.isInteger(value) && value > 0) }
function formatNumber(value: number): string { return new Intl.NumberFormat('zh-CN', { notation: value >= 100000 ? 'compact' : 'standard', maximumFractionDigits: 1 }).format(value) }
function formatBytes(value: number): string { return value >= 1048576 ? `${(value / 1048576).toFixed(1)} MiB` : `${Math.ceil(value / 1024)} KiB` }
function formatDate(value: string): string { return value ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium' }).format(new Date(value)) : '—' }
function formatRange(start: string, end: string): string { return start === end ? formatDate(start) : `${formatDate(start)}—${formatDate(end)}` }
function rankingPosition(row: UserInsightRanking): number { return (page.page - 1) * page.page_size + page.items.indexOf(row) + 1 }
function formatDateTime(value: string): string { return value ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'medium' }).format(new Date(value)) : '—' }
function formatDuration(value: number): string { return value > 0 ? `${formatNumber(value)} ms` : '—' }
function successRate(row: DailyInsight): string { const total = row.business_request_count + row.business_error_count; return total ? `${(row.business_request_count / total * 100).toFixed(1)}%` : '—' }
function sampleReasonLabel(value: string): string { return sampleReasonLabels[value] ?? `其他采样原因（${value || '未记录'}）` }
function triggerReasonLabel(value: string): string { return triggerReasonLabels[value] ?? `其他触发原因（${value || '未记录'}）` }
function statusLabel(value: string): string { return statusLabels[value] ?? `未知状态（${value || '未记录'}）` }
function logUserLabel(username: string, userID?: number): string { return userID ? (username || `用户 ${userID}`) : (username ? `${username}（用户已删除）` : '用户已删除') }
function errorLabel(value: string): string {
  if (errorLabels[value]) return errorLabels[value]
  if (value === 'analyzer_http_400') return '分析模型拒绝了请求（HTTP 400）'
  if (value === 'analyzer_http_401') return '分析模型 API Key 无效（HTTP 401）'
  if (value === 'analyzer_http_403') return '分析模型拒绝访问（HTTP 403）'
  if (value === 'analyzer_http_404') return '分析模型或接口地址不存在（HTTP 404）'
  if (value === 'analyzer_http_429') return '分析模型请求过于频繁（HTTP 429）'
  if (value.startsWith('analyzer_http_5')) return `分析模型服务端异常（${value.replace('analyzer_http_', 'HTTP ')}）`
  return `处理异常（错误码：${value}）`
}
function sampleErrorLabel(value: string): string {
  if (value === 'summary_write_failed') return '所属分析批次摘要写入数据库失败'
  if (value === 'summary_write_conflict') return '所属分析批次摘要写入版本冲突'
  return errorLabel(value)
}
function showMessage(text: string, error = false) { message.value = text; messageError.value = error }

async function loadConfig() { const value = await api.getConfig(); savedConfig.value = clone(value); draft.value = clone(value) }
async function loadRuntime() { runtime.value = await api.getRuntime() }
async function loadDaily() { Object.assign(page, await api.listRanking(appliedFilters.value, page.page, page.page_size)) }
async function loadOverview() { overview.value = await api.getOverview(appliedFilters.value) }
async function refresh() {
  loading.value = true
  try { const [, , , , accounts] = await Promise.all([loadConfig(), loadRuntime(), loadDaily(), loadOverview(), api.listAnalyzerAccounts()]); analyzerAccounts.value = accounts; message.value = '' }
  catch (error) { showMessage(error instanceof Error ? error.message : '数据加载失败', true) }
  finally { loading.value = false }
}
async function search() { appliedFilters.value = { ...filters }; page.page = 1; loading.value = true; try { await Promise.all([loadDaily(), loadOverview()]) } catch { showMessage('洞察列表加载失败', true) } finally { loading.value = false } }
async function changePage(value: number) { page.page = value; await loadDaily() }
async function changePageSize(value: number) { page.page_size = value; page.page = 1; await loadDaily() }
async function openDetail(row: UserInsightRanking) { detail.value = null; showAllSamples.value = false; detailOpen.value = true; try { detail.value = await api.getDaily(row.latest_insight_id) } catch { detailOpen.value = false; showMessage('洞察详情加载失败', true) } }
function resetConfig() { if (savedConfig.value) draft.value = clone(savedConfig.value) }
async function saveConfig() {
  if (!draft.value) return
  saving.value = true
  try { const value = await api.updateConfig(draft.value); savedConfig.value = clone(value); draft.value = clone(value); showMessage('AI 使用洞察配置已保存') }
  catch (error) { showMessage(error instanceof Error ? error.message : '配置保存失败，请刷新后重试', true) }
  finally { saving.value = false }
}
async function probeAnalyzer() {
  if (!draft.value) return
  probing.value = true
  try { const result = await api.probe(draft.value); showMessage(`${result.message}（${result.latency_ms} ms）`, !result.ok) }
  catch { showMessage('分析节点连接失败', true) }
  finally { probing.value = false }
}
async function analyzeNow() {
  analyzing.value = true
  try { const result = await api.analyzeNow(); showMessage(result.created_batches ? `已创建 ${result.created_batches} 个分析批次，后台正在处理` : '当前没有待分析样本'); await loadRuntime() }
  catch (error) { showMessage(error instanceof Error ? error.message : '立即分析失败', true) }
  finally { analyzing.value = false }
}
async function loadSampleLogs() { Object.assign(sampleLogPage, await api.listSamples(sampleLogPage.page, sampleLogPage.page_size)) }
async function loadPendingLogs() { Object.assign(pendingLogPage, await api.listBatches(pendingLogPage.page, pendingLogPage.page_size, 'pending')) }
async function loadProcessingLogs() { Object.assign(processingLogPage, await api.listBatches(processingLogPage.page, processingLogPage.page_size, 'processing')) }
async function loadCompletedLogs() { Object.assign(completedLogPage, await api.listBatches(completedLogPage.page, completedLogPage.page_size, 'done')) }
async function loadErrorLogs() { Object.assign(errorLogPage, await api.listBatches(errorLogPage.page, errorLogPage.page_size, 'errors')) }
async function loadBatchLogs() { await Promise.all([loadPendingLogs(), loadProcessingLogs(), loadCompletedLogs(), loadErrorLogs()]) }
async function loadLogs() { await Promise.all([loadSampleLogs(), loadBatchLogs()]) }
function stopLogRefresh() { if (logRefreshTimer) clearTimeout(logRefreshTimer); logRefreshTimer = undefined }
function scheduleLogRefresh() {
  stopLogRefresh()
  if (!logsOpen.value || !runtime.value || !(runtime.value.queued_batches + runtime.value.processing_batches + runtime.value.retry_batches)) return
  logRefreshTimer = setTimeout(async () => {
    if (!logsOpen.value) return
    try { await Promise.all([loadBatchLogs(), loadRuntime()]) }
    catch { stopLogRefresh(); return }
    scheduleLogRefresh()
  }, 2500)
}
function closeLogs() { logsOpen.value = false; stopLogRefresh() }
async function openLogs() {
  logsOpen.value = true
  logsLoading.value = true
  try { await Promise.all([loadLogs(), loadRuntime()]); scheduleLogRefresh() }
  catch { showMessage('采样与分析日志加载失败', true) }
  finally { logsLoading.value = false }
}
async function changeLogPage(target: LogPage<unknown>, value: number, load: () => Promise<void>) {
  target.page = value
  logsLoading.value = true
  try { await load() }
  catch { showMessage('日志分页加载失败', true) }
  finally { logsLoading.value = false }
}
async function changeLogPageSize(target: LogPage<unknown>, value: number, load: () => Promise<void>) {
  target.page_size = value
  target.page = 1
  logsLoading.value = true
  try { await load() }
  catch { showMessage('日志分页加载失败', true) }
  finally { logsLoading.value = false }
}
async function clearLogs() {
  if (clearingLogs.value) return
  clearingLogs.value = true
  try {
    const result = await api.clearLogs()
    clearLogsConfirm.value = false
    sampleLogPage.page = pendingLogPage.page = processingLogPage.page = completedLogPage.page = errorLogPage.page = 1
    await Promise.all([openLogs(), loadRuntime()])
    showMessage(`已清空 ${result.samples} 条采样日志和 ${result.batches} 条分析日志`)
  } catch (error) { showMessage(error instanceof Error ? error.message : '清空日志失败', true) }
  finally { clearingLogs.value = false }
}
async function retryBatch(item: BatchSummary) {
  if (retryingBatchID.value !== null) return
  retryingBatchID.value = item.id
  try {
    await api.retryBatch(item.id)
    await Promise.all([loadBatchLogs(), loadRuntime()])
    batchLogTab.value = 'pending'
    scheduleLogRefresh()
    showMessage('任务已进入异步队列，通常数秒内开始分析')
  } catch (error) { showMessage(error instanceof Error ? error.message : '重新分析失败', true) }
  finally { retryingBatchID.value = null }
}
async function retryAllBatches() {
  if (retryingAll.value) return
  retryingAll.value = true
  try {
    const result = await api.retryAllBatches()
    errorLogPage.page = 1
    await Promise.all([loadBatchLogs(), loadRuntime()])
    batchLogTab.value = 'pending'
    scheduleLogRefresh()
    showMessage(`已重新排队 ${result.retried} 条错误任务${result.skipped ? `，另有 ${result.skipped} 条因临时文本已过期而跳过` : ''}`)
  } catch (error) { showMessage(error instanceof Error ? error.message : '批量重新分析失败', true) }
  finally { retryingAll.value = false }
}
async function stopBatch(item: BatchSummary) {
  if (stoppingBatchID.value !== null) return
  stoppingBatchID.value = item.id
  try {
    await api.stopBatch(item.id)
    await Promise.all([loadBatchLogs(), loadRuntime()])
    batchLogTab.value = 'pending'
    showMessage('分析任务已暂停并移至待分析')
  } catch (error) { showMessage(error instanceof Error ? error.message : '停止分析失败', true) }
  finally { stoppingBatchID.value = null }
}

onMounted(refresh)
onUnmounted(stopLogRefresh)
</script>

<style scoped>
.field { @apply flex min-w-0 flex-col gap-1.5 text-sm text-gray-700 dark:text-dark-200; }
.field > span { @apply text-xs font-medium text-gray-600 dark:text-dark-300; }
.field small, label small { @apply mt-1 block text-xs font-normal text-gray-400; }
.form-grid { @apply grid gap-4 sm:grid-cols-2; }
.badge { @apply inline-flex rounded-md border border-gray-200 bg-gray-50 px-2 py-1 text-xs text-gray-700 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200; }
.mini-stat { @apply rounded-lg border border-gray-200 p-3 dark:border-dark-700; }
.mini-stat span { @apply block text-xs text-gray-500; }
.mini-stat strong { @apply mt-1 block text-lg text-gray-950 dark:text-white; }
.log-stat { @apply flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2 text-gray-500 dark:bg-dark-800; }
.log-stat strong { @apply text-base text-gray-950 dark:text-white; }
.section-title { @apply text-sm font-semibold text-gray-950 dark:text-white; }
aside dt { @apply text-xs text-gray-500; }
aside dd { @apply mt-1 font-semibold text-gray-950 dark:text-white; }
</style>
