<template>
  <AppLayout>
    <div class="analytics-page" :aria-busy="loading">
      <header class="analytics-heading">
        <div>
          <div class="analytics-eyebrow">USAGE INTELLIGENCE</div>
          <h1>看清用量，也看清投入。</h1>
          <p class="analytics-subtitle">按年、按月或自选多月统计，从部门下钻到每个人。</p>
        </div>
        <div class="analytics-toolbar">
          <div class="analytics-segmented" role="group" aria-label="时间范围模式">
            <button v-for="option in periodOptions" :key="option.value" type="button" :class="{ active: mode===option.value }" :aria-pressed="mode===option.value" @click="setMode(option.value)">{{ option.label }}</button>
          </div>
          <Select v-model="year" class="analytics-year-select" aria-label="统计年份" :options="yearOptions" @change="changePeriod" />
          <Select v-if="mode==='month'" v-model="month" class="analytics-month-select" aria-label="统计月份" :options="monthOptions" @change="changePeriod" />
          <button v-if="mode==='months'" type="button" class="analytics-button" aria-label="选择统计月份" @click="openMonths">{{ months.join('、') }} 月 <Icon name="chevronDown" size="sm" /></button>
          <button type="button" class="analytics-icon-button" aria-label="刷新分析数据" :disabled="loading" @click="load(true)"><Icon name="refresh" :class="{ 'animate-spin': loading }" /></button>
          <button type="button" class="analytics-button primary" :disabled="!report || loading || exporting || !enabled || !report.coverage.directory_ready" @click="download"><Icon name="download" size="sm" />{{ exporting ? '正在准备…' : '导出报表' }}</button>
        </div>
      </header>

      <div class="analytics-notice">
        <div v-if="directory?.version"><strong>真实组织与人员</strong><span>{{ directory.directory.orgs.length }} 个组织、{{ directory.directory.people.length }} 名人员；仅展示三级，下级用量与成本自动归集。</span></div>
        <div v-else><strong>先维护组织与人员目录</strong><span>上传并应用两份 SQL 后，后台开始分批统计真实 Token 与采购成本。</span></div>
        <button type="button" class="analytics-text-button" @click="setTab('maintenance')">{{ directoryView.pending ? `${directoryView.pending} 个平台用户待确认` : '维护组织与人员' }} <Icon name="arrowRight" size="sm" /></button>
      </div>
      <p v-if="error" role="alert" class="analytics-alert error">{{ error }}<button type="button" class="analytics-text-button" @click="load(true)">重新加载</button></p>

      <AnalyticsProgress @loaded="taskStatus=$event" />

      <nav class="analytics-tabs" role="tablist" aria-label="数据分析视图">
        <button v-for="item in tabs" :id="`analytics-tab-${item.id}`" :key="item.id" type="button" role="tab" :class="{ active: tab===item.id }" :aria-selected="tab===item.id" :aria-controls="`analytics-panel-${item.id}`" :tabindex="tab===item.id ? 0 : -1" @click="setTab(item.id)" @keydown="tabKeydown($event,item.id)">
          {{ item.label }}<span v-if="item.id==='people' && directory" class="analytics-count">{{ directory.directory.people.length }}</span><span v-if="item.id==='maintenance' && directoryView.pending" class="analytics-count">{{ directoryView.pending }}</span>
        </button>
      </nav>

      <div v-if="tab==='org' || tab==='people'" :id="`analytics-panel-${tab}`" role="tabpanel" :aria-labelledby="`analytics-tab-${tab}`">
        <div class="analytics-scope-line">
          <nav class="analytics-crumbs" aria-label="组织范围">
            <button type="button" :aria-current="!org ? 'page' : undefined" @click="selectOrg('')">全部组织</button>
            <template v-for="crumb in breadcrumbs" :key="crumb.id"><span>/</span><button type="button" :aria-current="crumb.id===org ? 'page' : undefined" @click="selectOrg(crumb.id)">{{ crumb.name }}</button></template>
          </nav>
          <span>{{ rangeLabel }} · 趋势{{ mode==='month' ? '按天' : '按月' }} · 北京时间</span>
        </div>

        <div v-if="report && !report.coverage.complete" class="analytics-alert warning" role="status">
          <span v-if="!report.coverage.directory_ready">请先在「数据维护」预检并应用两份 SQL。启用前已经清理的原始历史无法恢复。</span>
          <template v-else>所选月份已完成 {{ report.coverage.completed_days }} / {{ report.coverage.expected_days }} 天；价格待补 {{ report.coverage.pending_requests }} 条请求。当前展示已发布结果，总体任务进度见上方。</template>
          <p v-if="report.coverage.history_gap">可用请求起于 {{ report.coverage.raw_history_start?.slice(0,10) }}，此前已清理的用量无法恢复。</p>
          <p v-for="issue in report.coverage.issues" :key="issue.day">{{ issue.day }}：{{ issue.message }}</p>
        </div>
        <p v-if="report && !enabled" class="analytics-paused"><Icon name="clock" size="sm" />后台统计已暂停，页面保留已发布结果。<a href="/admin/settings">前往系统设置</a></p>

        <section class="analytics-stats" aria-label="使用与成本指标" aria-live="polite">
          <article v-for="card in cards" :key="card.label" class="analytics-panel analytics-stat">
            <Icon :name="card.icon" class="analytics-stat-icon" />
            <span class="analytics-stat-label">{{ card.label }}</span><strong>{{ report ? card.value : '—' }}</strong><p>{{ card.note }}</p>
          </article>
        </section>

        <section class="analytics-overview">
          <article class="analytics-panel analytics-trend">
            <div class="analytics-panel-heading"><h2>{{ mode==='month' ? '每日' : '每月' }} Token 使用趋势</h2><span>{{ rangeLabel }}</span></div>
            <div v-if="hasTrend" class="analytics-chart">
              <canvas ref="canvas" role="img" :aria-label="`${rangeLabel} Token使用趋势`"></canvas>
              <div v-if="hoveredTrend" class="analytics-trend-tooltip" :class="{'on-left':tooltipOnLeft}" tabindex="0" role="region" aria-label="Token 模型明细" @keydown.esc.stop="hoveredTrend=undefined">
                <div class="analytics-tooltip-row"><strong>{{ hoveredTrend.bucket }}</strong><button type="button" aria-label="关闭 Token 模型明细" @click="hoveredTrend=undefined"><Icon name="x" size="sm" /></button></div>
                <p v-if="hoveredTrend.models?.length" class="analytics-tooltip-hint">{{ hoveredTrend.models.length }} 项模型明细 · 按用量降序 · 可滚动查看全部</p>
                <div v-for="series in trendSeries" :key="series.key" class="analytics-tooltip-group">
                  <div class="analytics-tooltip-row"><span><i :style="{background:series.color}"></i>{{ series.label }}</span><strong>{{ hoveredTrend[series.key]===undefined ? '待补算' : tokenText(hoveredTrend[series.key]) }}</strong></div>
                  <div v-for="model in hoveredModels.filter(model=>model.mode===series.mode)" :key="model.model" class="analytics-tooltip-row analytics-tooltip-model"><span>{{ model.model }}</span><span>{{ tokenText(model.tokens) }}</span></div>
                </div>
                <p v-if="hoveredTrend.metered_tokens===undefined">分类与模型明细待后台补算</p>
              </div>
            </div>
            <div v-else class="analytics-chart analytics-empty"><Icon name="trendingUp" size="xl" /><strong>{{ loading ? '正在加载用量趋势' : '当前范围暂无用量' }}</strong><span>{{ report?.coverage.directory_ready ? '完成后台核算后，趋势会随所选时间显示。' : '应用组织与人员目录后开始统计。' }}</span></div>
            <p class="analytics-bottom-note">{{ rangeLabel }} · 悬停查看各方案与模型 Token<span v-if="mode!=='month'"> · 点击月份查看每日趋势</span><span v-if="report?.coverage.provisional"> · 当前月为暂估</span><span v-if="report?.trend.some(point=>point.metered_tokens===undefined && point.tokens!=='0')"> · 部分历史分类明细待补算</span></p>
          </article>
          <article class="analytics-panel analytics-ranking">
            <div class="analytics-panel-heading"><h2>部门用量排行</h2><span>Token · TOP 5</span></div>
            <div v-if="ranking.length" class="analytics-ranking-list">
              <button v-for="(item,index) in ranking" :key="item.id" type="button" class="analytics-rank-row" @click="selectOrg(item.id,true)">
                <span class="analytics-rank-index">{{ String(index+1).padStart(2,'0') }}</span><span class="analytics-rank-main"><span>{{ item.name }}</span><span class="analytics-rank-track"><i :style="{width:`${tokenShare(item.usage?.tokens,ranking[0]?.usage?.tokens)}%`}"></i></span></span><span class="analytics-rank-value">{{ tokenText(item.usage?.tokens) }}</span>
              </button>
            </div>
            <div v-else class="analytics-empty analytics-ranking-empty"><Icon name="chartBar" size="xl" /><span>当前范围暂无部门用量</span></div>
            <p class="analytics-bottom-note">按所选月份合计 · 点击部门查看人员</p>
          </article>
        </section>

        <section class="analytics-details-grid">
          <aside class="analytics-panel analytics-org-panel">
            <div class="analytics-detail-heading">
              <div><h2>组织架构</h2><p>公司 → 中心 / 团队 → 三级部门</p></div>
              <div class="analytics-search"><Icon name="search" size="sm" /><input v-model="orgSearch" aria-label="搜索组织" placeholder="搜索组织名称"></div>
            </div>
            <div class="analytics-tree" aria-label="组织架构">
              <div class="analytics-tree-row" :class="{ selected: org==='' }"><span class="analytics-tree-toggle"></span><button type="button" class="analytics-tree-label" :aria-current="org==='' ? 'true' : undefined" @click="selectOrg('')"><span><Icon name="grid" size="sm" />全部组织</span><small>{{ directory?.directory.people.length || 0 }}</small></button></div>
              <div v-for="node in treeRows.slice(0,300)" :key="node.id" class="analytics-tree-row" :class="{ selected: org===node.id }" :style="{paddingLeft:`${(node.level-1)*14}px`}">
                <button v-if="node.hasChildren" type="button" class="analytics-tree-toggle" :aria-label="`${expanded.has(node.id) ? '收起' : '展开'}${node.name}`" :aria-expanded="expanded.has(node.id) || !!orgSearch" @click="toggleNode(node.id)"><Icon :name="expanded.has(node.id) || orgSearch ? 'chevronDown' : 'chevronRight'" size="xs" /></button>
                <span v-else class="analytics-tree-toggle"></span>
                <button type="button" class="analytics-tree-label" :aria-current="org===node.id ? 'true' : undefined" @click="selectOrg(node.id)"><span><small v-if="node.level<3" class="analytics-level">{{ node.level }}级</small>{{ node.name }}</span><small>{{ directoryView.counts.get(node.id) || 0 }}</small></button>
              </div>
              <p v-if="treeRows.length>300" class="analytics-tree-empty">组织较多，当前显示前 300 项；搜索可定位其他组织。</p>
              <p v-if="orgSearch && !treeRows.length" class="analytics-tree-empty">没有匹配的组织</p>
              <div v-if="!orgSearch" class="analytics-tree-row analytics-pending" :class="{ selected: org==='0' }"><span class="analytics-tree-toggle"></span><button type="button" class="analytics-tree-label" :aria-current="org==='0' ? 'true' : undefined" @click="selectOrg('0')"><span><Icon name="exclamationCircle" size="sm" />待确认归属</span><small>{{ directoryView.pending }}</small></button></div>
            </div>
            <p class="analytics-bottom-note">人数来自最新目录，包含下级人员。历史用量按发生时组织归集。</p>
          </aside>

          <article class="analytics-panel analytics-detail-table">
            <div class="analytics-detail-heading">
              <div><h2>{{ tab==='people' ? '人员使用与成本' : '各部门使用情况' }}</h2><p>{{ tab==='people' ? '点击姓名，查看模型用量、成本组成和全年逐月明细' : '上级包含全部下级，四级及以下归入对应三级组织' }}</p></div>
              <div class="analytics-table-controls">
                <div class="analytics-search"><Icon name="search" size="sm" /><input v-if="tab==='org'" v-model="departmentSearch" aria-label="搜索部门明细" placeholder="搜索部门名称"><input v-else v-model="peopleSearch" aria-label="搜索人员明细" placeholder="搜索人员姓名" @keyup.enter="searchPeople"></div>
                <Select v-if="tab==='org'" v-model="depth" class="analytics-depth-select" aria-label="组织展示层级" :options="depthOptions" />
                <Select v-model="sort" class="analytics-sort-select" aria-label="统计排序" :options="sortOptions" @change="sortChanged" />
              </div>
            </div>
            <div class="analytics-table-scroll" :aria-busy="loading">
              <table class="analytics-table" :aria-label="tab==='people' ? '人员Token与实际成本' : '部门Token与实际成本'">
                <colgroup><col style="width:32%"><col style="width:17%"><col style="width:18%"><col style="width:17%"><col style="width:16%"></colgroup>
                <thead><tr><th>{{ tab==='people' ? '人员 / 来源组织' : '组织名称' }}</th><th :class="{num:tab==='org'}">{{ tab==='people' ? '归属状态' : '目录人数 / 使用' }}</th><th class="num">Token 总数</th><th class="num">实际成本</th><th class="num">Token 占比</th></tr></thead>
                <tbody v-if="tab==='org'">
                  <tr v-for="item in visibleDepartments" :key="item.id" :class="{ 'analytics-special-row':item.id==='0' }">
                    <td><button type="button" class="analytics-table-link" @click="selectOrg(item.id, org===item.id)"><span>{{ item.name }}</span><Icon name="arrowRight" size="xs" /></button><small class="analytics-subcell">{{ item.id==='0' ? '保留未关联人员的全部用量与成本' : `${item.level}级组织 · 含下级` }}</small></td>
                    <td class="num"><span class="analytics-people-count"><span>{{ item.id==='0' ? '—' : directoryView.counts.get(item.id) || 0 }}</span><span>/</span><span>{{ item.usage?.users || 0 }}</span></span></td><td class="num">{{ tokenText(item.usage?.tokens) }}</td><td class="num analytics-total">{{ moneyText(item.usage?.cost) }}</td>
                    <td class="num"><span class="analytics-share"><span class="analytics-mini-track"><i :style="{width:`${tokenShare(item.usage?.tokens,report?.summary.tokens)}%`}"></i></span>{{ tokenShare(item.usage?.tokens,report?.summary.tokens).toFixed(1) }}%</span></td>
                  </tr>
                  <tr v-if="!visibleDepartments.length"><td colspan="5" class="analytics-table-empty">{{ loading ? '正在加载部门统计…' : '当前范围暂无部门，尝试更换组织或维护目录。' }}</td></tr>
                </tbody>
                <tbody v-else>
                  <tr v-for="person in report?.people || []" :key="person.user_id">
                    <td><button type="button" class="analytics-table-link" @click="selectedPerson=person"><span>{{ person.name }}</span><Icon name="arrowRight" size="xs" /></button><small class="analytics-subcell">{{ person.org_names.slice(0,3).join(' / ') || '待确认归属' }}</small></td>
                    <td><span class="analytics-pill" :class="{warn:!person.person_id || (!person.org_ids.length && !person.org_names.length)}">{{ person.person_id && person.org_ids.length ? '已归属' : person.org_names.length && !person.org_ids.length ? '多组织明细' : '待确认' }}</span></td>
                    <td class="num">{{ tokenText(person.tokens) }}</td><td class="num analytics-total">{{ moneyText(person.cost) }}</td>
                    <td class="num"><span class="analytics-share"><span class="analytics-mini-track"><i :style="{width:`${tokenShare(person.tokens,report?.summary.tokens)}%`}"></i></span>{{ tokenShare(person.tokens,report?.summary.tokens).toFixed(1) }}%</span></td>
                  </tr>
                  <tr v-if="!report?.people.length"><td colspan="5" class="analytics-table-empty">{{ loading ? '正在加载人员统计…' : '当前范围暂无人员用量，或没有匹配的姓名。' }}</td></tr>
                </tbody>
              </table>
            </div>
            <div v-if="tab==='org'" class="analytics-table-footer"><span>共 {{ departmentRows.length }} 个组织 · 各层级包含下级，请勿跨级相加</span><div><button type="button" :disabled="departmentPage===1" aria-label="上一页部门" @click="departmentPage--"><Icon name="chevronLeft" size="sm" /></button><span>{{ departmentPage }} / {{ departmentPages }}</span><button type="button" :disabled="departmentPage>=departmentPages" aria-label="下一页部门" @click="departmentPage++"><Icon name="chevronRight" size="sm" /></button></div></div>
            <Pagination v-else :page="page" :page-size="pageSize" :total="report?.people_total || 0" @update:page="page=$event;load()" @update:pageSize="pageSize=$event;resetAndLoad()" />
          </article>
          <div v-if="report" class="analytics-reconcile">
            <div v-if="!org"><strong>{{ reconciled ? '汇总一致' : '统计核算中' }}</strong><span>部门 Token {{ tokenText(assignedTokens) }} ＋ 待确认 {{ tokenText(pendingDepartment?.tokens) }} ＝ {{ tokenText(report.summary.tokens) }}</span><span>实际总成本 {{ moneyText(report.summary.cost) }}，其中未分摊公共成本 {{ moneyText(report.public_cost) }}</span></div>
            <div v-else><strong>当前组织包含全部下级</strong><span>Token {{ tokenText(report.summary.tokens) }} · 实际成本 {{ moneyText(report.summary.cost) }}</span><button type="button" class="analytics-text-button" @click="selectOrg('')">返回全部组织 <Icon name="arrowRight" size="xs" /></button></div>
            <span>最近核算：{{ report.coverage.last_updated ? new Date(report.coverage.last_updated).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai'}) : '尚未核算' }}</span>
          </div>
        </section>
      </div>

      <div v-else-if="tab==='maintenance'" id="analytics-panel-maintenance" role="tabpanel" aria-labelledby="analytics-tab-maintenance">
        <DirectoryPanel :enabled="enabled" :initial-state="directory" @loaded="directory=$event" @changed="directoryChanged" />
      </div>
      <div v-else id="analytics-panel-rules" role="tabpanel" aria-labelledby="analytics-tab-rules">
        <div class="analytics-rules">
          <article class="analytics-panel"><h2>01 Token 总数与时间范围</h2><p><strong>所有模型都统计</strong>输入＋输出＋缓存写入＋缓存读取，包含不计成本模型。统一使用 B / M / K，非零不足 1K 显示「＜1K」。</p><p><strong>趋势跟随顶部时间</strong>按年看每月，按月看每天，多月只对比所选月份。当前月只统计已发生的日期。</p><div class="analytics-formula">所选月份总数 = 各所选月份 Token 之和</div><p>使用人数按平台规范用户去重；同一个人跨月使用只计一人。导出覆盖当前组织和所选月份，不受个人表分页、姓名搜索影响。</p></article>
          <article class="analytics-panel"><h2>02 费用就是实际采购成本</h2><p><strong>按量成本</strong>取请求发生时生效的采购价格，{{ report ? `当前范围 ${moneyText(report.summary.metered)}。` : '以已发布成本账为准。' }}</p><p><strong>固定成本</strong>订阅按有效期计提，再由当月实际使用过该订阅的人员均摊；同一个人使用同订阅多个账号只计一人，未使用者不分摊。</p><div class="analytics-formula">实例当月计提成本 ÷ 当月去重使用人数</div><p>无人使用的成本进入公共池，只计公司总成本。当前月份的计提金额和使用人数变化时，个人份额会重算。</p></article>
          <article class="analytics-panel"><h2>03 组织归集与身份关联</h2><p><strong>只展示第一级至第三级</strong>更深组织的用量与成本计入第三级；上级包含下级，不能跨级相加。目录人数来自最新目录，用量归属以发生时组织为准。</p><p><strong>优先稳定 ID</strong>复用外部人员关联，其次双方唯一的规范化中文姓名；同名、缺少组织和未关联人员都保留在待确认池，手动确认可在「数据维护」完成。</p><div class="analytics-formula">已归属部门成本＋待确认成本＋公共固定成本 = 总成本</div></article>
          <article class="analytics-panel"><h2>04 目录维护与历史数据</h2><p><strong>两表统一应用</strong>上传 devp_org、devp_user 的 SQL 数据，预检后一起应用。SQL 仅作为文本读取，保留必要字段，保存目录版本和归属生效时间。</p><p>{{ report?.coverage.history_basis || '首次目录按当前组织回溯；后续按实际生效时间归属。' }} 已清理的请求无法恢复，历史不完整或价格未配置会明确标注。</p><p>日模型明细保留 13 个月，月模型统计和部门每日趋势保留 60 个月；后台按批次处理，可在系统设置暂停。</p></article>
        </div>
        <section class="analytics-panel analytics-maintenance-card analytics-rebuild">
          <h2>历史数据补算</h2><p class="analytics-muted">仅对仍有完整原始请求的范围补算，每次最多 31 天；已归档且无法重建的期间保持原发布结果。</p>
          <div class="analytics-toolbar"><label>开始日期<input v-model="rebuildStart" type="date" class="input"></label><label>结束日期<input v-model="rebuildEnd" type="date" class="input"></label><button type="button" class="analytics-button" :disabled="!enabled || rebuilding" @click="rebuild">{{ rebuilding ? '提交中…' : '提交补算任务' }}</button></div>
        </section>
      </div>
      <p class="analytics-page-footer">Token 与人民币采购成本 · {{ report?.coverage.provisional ? '本月为暂估，以已发布结果为准' : '基于已发布统计结果' }}</p>
    </div>

    <BaseDialog :show="monthPickerOpen" title="选择统计月份" @close="monthPickerOpen=false">
      <p class="mb-4 text-sm text-gray-500">可选择不连续月份，所有指标只统计所选月份。</p>
      <div class="analytics-month-grid"><label v-for="option in monthOptions" :key="option.value" :class="{selected:draftMonths.includes(option.value)}"><input v-model="draftMonths" type="checkbox" :value="option.value">{{ option.label }}</label></div>
      <p v-if="!draftMonths.length" class="mt-3 text-sm text-amber-600">请至少选择一个月份。</p>
      <template #footer><button type="button" class="btn btn-secondary" @click="monthPickerOpen=false">取消</button><button type="button" class="btn btn-primary" :disabled="!draftMonths.length" @click="applyMonths">应用所选月份</button></template>
    </BaseDialog>
    <PersonDrawer :person="selectedPerson" :query="appliedQuery" :versions="report?.versions || {}" :range-label="rangeLabel" @close="selectedPerson=undefined" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { Chart } from 'chart.js/auto'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import Select from '@/components/common/Select.vue'
import Pagination from '@/components/common/Pagination.vue'
import AnalyticsProgress from './AnalyticsProgress.vue'
import DirectoryPanel from './DirectoryPanel.vue'
import PersonDrawer from './PersonDrawer.vue'
import { dataAnalyticsAPI, type AnalyticsDirectoryState, type AnalyticsQuery, type AnalyticsReport, type AnalyticsRow, type AnalyticsTaskStatus, type AnalyticsTrend } from '@/api/admin/dataAnalytics'
import { useAppStore } from '@/stores/app'
import { moneyText, tokenText, sumDecimalStrings } from './format'
import { organizationDirectory, tokenShare } from './organizations'
import { exportAnalytics } from './export'
import './analytics.css'

type Tab = 'org' | 'people' | 'maintenance' | 'rules'
const app = useAppStore()
const now = new Date(new Date().toLocaleString('en-US', { timeZone: 'Asia/Shanghai' }))
const year = ref(now.getFullYear()), mode = ref('month'), month = ref(now.getMonth()+1), months = ref([now.getMonth()+1])
const org = ref(''), tab = ref<Tab>('org'), orgSearch = ref(''), departmentSearch = ref(''), peopleSearch = ref('')
const depth = ref('children'), sort = ref('tokens'), page = ref(1), pageSize = ref(12), departmentPage = ref(1)
const report = shallowRef<AnalyticsReport>(), directory = shallowRef<AnalyticsDirectoryState>(), appliedQuery = shallowRef<AnalyticsQuery>()
const taskStatus = shallowRef<AnalyticsTaskStatus | null>()
const loading = ref(false), exporting = ref(false), error = ref(''), selectedPerson = shallowRef<AnalyticsRow>()
const monthPickerOpen = ref(false), draftMonths = ref<number[]>([])
const rebuildStart = ref(''), rebuildEnd = ref(''), rebuilding = ref(false), canvas = ref<HTMLCanvasElement>()
const hoveredTrend = shallowRef<AnalyticsTrend>(), tooltipOnLeft = ref(false)
const hoveredModels = computed(() => [...(hoveredTrend.value?.models || [])].sort((a,b) => {
  const first=BigInt(a.tokens), second=BigInt(b.tokens)
  return first===second ? a.model.localeCompare(b.model) : first>second ? -1 : 1
}))
const trendSeries = [
  {key:'tokens',label:'Token 总数',color:'#0d9488',mode:''},
  {key:'metered_tokens',label:'按量方案',color:'#3b82f6',mode:'metered'},
  {key:'fixed_tokens',label:'固定成本',color:'#a855f7',mode:'fixed'},
  {key:'excluded_tokens',label:'不纳入核算',color:'#f59e0b',mode:'excluded'},
] as const
const expanded = ref(new Set<string>())
let chart: Chart | undefined, abort: AbortController | undefined, exportAbort: AbortController | undefined
let timer: ReturnType<typeof setInterval> | undefined, searchTimer: ReturnType<typeof setTimeout> | undefined, sequence = 0, disposed = false
let requestedSearch = ''
const tabs: {id:Tab;label:string}[] = [{id:'org',label:'部门分析'},{id:'people',label:'人员分析'},{id:'maintenance',label:'数据维护'},{id:'rules',label:'统计口径'}]
const periodOptions = [{value:'year',label:'按年'},{value:'month',label:'按月'},{value:'months',label:'多月'}]
const yearOptions = Array.from({length:6},(_,i)=>({value:now.getFullYear()-i,label:`${now.getFullYear()-i} 年${i===0?'（今年）':''}`}))
const cutoff = new Date(now.getFullYear(),now.getMonth()-59,1)
const monthOptions = computed(()=>Array.from({length:year.value===now.getFullYear()?now.getMonth()+1:12},(_,i)=>({value:i+1,label:`${i+1} 月`})).filter(m=>new Date(year.value,m.value-1,1)>=cutoff))
const depthOptions = [{value:'children',label:'直属下级'},{value:'3',label:'展开至三级'}]
const sortOptions = [{value:'tokens',label:'Token 总数 ↓'},{value:'cost',label:'实际成本 ↓'},{value:'name',label:'名称排序'}]
const enabled = computed(()=>taskStatus.value?.control.enabled ?? false)
const directoryView = computed(()=>organizationDirectory(directory.value,report.value?.departments))
const usageByOrg = computed(()=>new Map((report.value?.departments || []).map(row=>[row.id,row])))
const breadcrumbs = computed(()=>org.value==='0' ? [{id:'0',name:'待确认归属'}] : (directoryView.value.nodes.get(org.value)?.path || []).slice(0,3).map(id=>({id,name:directoryView.value.nodes.get(id)?.name || '历史组织'})))
const rangeLabel = computed(()=>{
  const period=report.value?.period
  if(!period)return `${year.value} 年`
  return period.mode==='year' && (period.year===now.getFullYear() || period.months.length===12) ? `${period.year} 年${period.year===now.getFullYear()?'累计':'全年'}` : `${period.year} 年 ${period.months.join('、')} 月`
})
const directoryCount = computed(()=>!org.value ? directory.value?.directory.people.length || 0 : org.value==='0' ? directoryView.value.pending : directoryView.value.counts.get(org.value) || 0)
const cards = computed(()=>[
  {label:'Token 总数',value:tokenText(report.value?.summary.tokens),note:'含按量、固定及不计成本模型',icon:'chartBar' as const},
  {label:'实际总成本',value:moneyText(report.value?.summary.cost),note:`按量 ${moneyText(report.value?.summary.metered)} · 固定 ${moneyText(report.value?.summary.fixed)}`,icon:'calculator' as const},
  {label:'使用人数',value:`${report.value?.users || 0} 人`,note:`当前组织目录 ${directoryCount.value} 人`,icon:'users' as const},
  {label:'人均 Token',value:tokenText(report.value?.users ? (BigInt(report.value.summary.tokens)/BigInt(report.value.users)).toString() : '0'),note:'按有使用记录的规范用户计算',icon:'trendingUp' as const},
])
const treeRows = computed(()=>{
  const nodes=[...directoryView.value.nodes.values()], children=new Map<string,typeof nodes>()
  for(const node of nodes){const parent=directoryView.value.nodes.has(node.parent)?node.parent:'';const list=children.get(parent)||[];list.push(node);children.set(parent,list)}
  const matching=new Set<string>(), query=orgSearch.value.trim().toLocaleLowerCase()
  if(query)for(const node of nodes)if(node.name.toLocaleLowerCase().includes(query))node.path.forEach(id=>matching.add(id))
  const result: (typeof nodes[number] & {hasChildren:boolean})[]=[], visited=new Set<string>()
  function walk(parent:string){for(const node of children.get(parent)||[]){if(visited.has(node.id) || (query&&!matching.has(node.id)))continue;visited.add(node.id);result.push({...node,hasChildren:!!children.get(node.id)?.length});if(query||expanded.value.has(node.id))walk(node.id)}}
  walk('');return result
})
const scopedDepartments = computed(()=>{
  const rows=[...directoryView.value.nodes.values()].filter(node=>!org.value || node.path.includes(org.value))
  return rows.map(node=>({...node,usage:usageByOrg.value.get(node.id)}))
})
const departmentRows = computed(()=>{
  let rows=scopedDepartments.value
  if(depth.value==='children'){
    let parent=org.value
    const roots=rows.filter(row=>!directoryView.value.nodes.has(row.parent))
    if(!parent&&roots.length===1)parent=roots[0]!.id
    const children=rows.filter(row=>row.parent===parent)
    rows=children.length ? children : rows.filter(row=>row.id===parent || !directoryView.value.nodes.has(row.parent))
  }else{
    const parents=new Set(rows.map(row=>row.parent))
    rows=rows.filter(row=>row.id!==org.value && (row.level>1 || !parents.has(row.id)))
  }
  if(org.value==='0' || (!org.value && (usageByOrg.value.has('0') || directoryView.value.pending))){
    rows=[...rows,{id:'0',parent:'',name:'待确认归属',level:1,path:['0'],names:['待确认归属'],usage:usageByOrg.value.get('0')}]
  }
  const query=departmentSearch.value.trim().toLocaleLowerCase()
  return rows.filter(row=>!query||row.name.toLocaleLowerCase().includes(query)).sort((a,b)=>{
    if(sort.value==='name')return a.name.localeCompare(b.name,'zh-CN')
    if(sort.value==='cost')return compareDecimal(b.usage?.cost,a.usage?.cost)
    const first=BigInt(a.usage?.tokens || '0'),second=BigInt(b.usage?.tokens || '0')
    return first===second ? a.name.localeCompare(b.name,'zh-CN') : first>second ? -1 : 1
  })
})
const departmentPages = computed(()=>Math.max(1,Math.ceil(departmentRows.value.length/10)))
const visibleDepartments = computed(()=>departmentRows.value.slice((departmentPage.value-1)*10,departmentPage.value*10))
const ranking = computed(()=>{
  const rows=scopedDepartments.value, parents=new Set(rows.map(row=>row.parent))
  return rows.filter(row=>!parents.has(row.id) && BigInt(row.usage?.tokens || '0')>0n).sort((a,b)=>BigInt(a.usage!.tokens)>BigInt(b.usage!.tokens)?-1:1).slice(0,5)
})
const hasTrend = computed(()=>!!report.value?.trend.some(point=>BigInt(point.tokens)>0n))
const pendingDepartment = computed(()=>usageByOrg.value.get('0'))
const rootDepartments = computed(()=>(report.value?.departments || []).filter(row=>row.level===1&&row.id!=='0'))
const assignedTokens = computed(()=>rootDepartments.value.reduce((sum,row)=>sum+BigInt(row.tokens),0n).toString())
const reconciled = computed(()=>!!report.value?.coverage.complete
  && BigInt(assignedTokens.value)+BigInt(pendingDepartment.value?.tokens || '0')===BigInt(report.value.summary.tokens)
  && compareDecimal(sumDecimalStrings([...rootDepartments.value.map(row=>row.cost),pendingDepartment.value?.cost || '0',report.value.public_cost]),report.value.summary.cost)===0)

function compareDecimal(a='0',b='0'){
  const units=(value:string)=>{const [whole='0',part='']=value.split('.');return BigInt(whole)*1000000000000n+BigInt(part.padEnd(12,'0').slice(0,12))}
  const first=units(a),second=units(b);return first===second ? 0 : first>second ? 1 : -1
}
function query():AnalyticsQuery{return {year:year.value,mode:mode.value,months:(mode.value==='month'?[month.value]:months.value).join(','),org_id:org.value,search:peopleSearch.value.trim(),page:page.value,page_size:pageSize.value,sort:sort.value}}
function setMode(value:string){if(value===mode.value)return;mode.value=value;changePeriod()}
function changePeriod(){
  const valid=monthOptions.value.map(option=>option.value)
  if(!valid.includes(month.value))month.value=valid[valid.length-1]!
  months.value=months.value.filter(value=>valid.includes(value))
  if(!months.value.length)months.value=[month.value]
  resetAndLoad()
}
function openMonths(){draftMonths.value=[...months.value];monthPickerOpen.value=true}
function applyMonths(){if(!draftMonths.value.length)return;months.value=[...draftMonths.value].sort((a,b)=>a-b);monthPickerOpen.value=false;resetAndLoad()}
function toggleNode(id:string){const next=new Set(expanded.value);if(next.has(id))next.delete(id);else next.add(id);expanded.value=next}
function selectOrg(id:string,people=false){
  org.value=id;peopleSearch.value='';departmentSearch.value='';selectedPerson.value=undefined
  const next=new Set(expanded.value);directoryView.value.nodes.get(id)?.path.forEach(parent=>next.add(parent));expanded.value=next
  if(people)tab.value='people'
  resetAndLoad()
}
function setTab(value:Tab){tab.value=value;selectedPerson.value=undefined;if(value==='people'&&appliedQuery.value?.sort!==sort.value)resetAndLoad()}
function tabKeydown(event:KeyboardEvent,id:Tab){
  let index=tabs.findIndex(item=>item.id===id)
  if(event.key==='ArrowRight')index=(index+1)%tabs.length
  else if(event.key==='ArrowLeft')index=(index+tabs.length-1)%tabs.length
  else if(event.key==='Home')index=0
  else if(event.key==='End')index=tabs.length-1
  else return
  event.preventDefault();setTab(tabs[index]!.id);document.getElementById(`analytics-tab-${tab.value}`)?.focus()
}
function resetAndLoad(){if(searchTimer)clearTimeout(searchTimer);page.value=1;departmentPage.value=1;selectedPerson.value=undefined;chart?.destroy();chart=undefined;report.value=undefined;void load()}
function searchPeople(){if(searchTimer)clearTimeout(searchTimer);resetAndLoad()}
function sortChanged(){departmentPage.value=1;if(tab.value==='people')resetAndLoad()}
watch(peopleSearch,()=>{if(searchTimer)clearTimeout(searchTimer);if(tab.value==='people'&&peopleSearch.value.trim()!==requestedSearch)searchTimer=setTimeout(searchPeople,350)})
watch([depth,departmentSearch],()=>{departmentPage.value=1})
watch(departmentPages,total=>{departmentPage.value=Math.min(departmentPage.value,total)})
watch(tab,async()=>{chart?.destroy();chart=undefined;await nextTick();if(!disposed)draw()})
watch(()=>directory.value?.version,()=>{const next=new Set(expanded.value);for(const node of directory.value?.directory.orgs || [])if(node.level<=2)next.add(node.id);expanded.value=next})
async function load(refreshDirectory=false){
  const current=++sequence, controller=new AbortController();abort?.abort();abort=controller;loading.value=true;error.value=''
  const params=query()
  requestedSearch=params.search || ''
  try{
    const data=await dataAnalyticsAPI.report(params,controller.signal)
    if(disposed||current!==sequence)return
    report.value=data;appliedQuery.value=params
    if(taskStatus.value===undefined)taskStatus.value=data.task_status
    if(refreshDirectory||!directory.value){const state=await dataAnalyticsAPI.directory();if(disposed||current!==sequence)return;directory.value=state}
    await nextTick();if(current===sequence&&!disposed)draw()
  }catch(cause){if(current===sequence&&!controller.signal.aborted)error.value=cause instanceof Error?cause.message:'报表加载失败'}
  finally{if(current===sequence)loading.value=false}
}
function draw(){
  chart?.destroy();chart=undefined;hoveredTrend.value=undefined
  if(!canvas.value||!report.value||!hasTrend.value||(tab.value!=='org'&&tab.value!=='people'))return
  const points=report.value.trend, daily=report.value.period.grain==='day'
  const primary=getComputedStyle(canvas.value).getPropertyValue('--analytics-green').trim() || '#0d9488'
  chart=new Chart(canvas.value,{type:'line',data:{labels:points.map(point=>daily?`${Number(point.bucket.slice(5,7))}/${Number(point.bucket.slice(8,10))}`:`${Number(point.bucket.slice(5,7))}月`),datasets:trendSeries.map(series=>({label:series.label,data:points.map(point=>point[series.key]===undefined?null:Number(point[series.key])),borderColor:series.key==='tokens'?primary:series.color,backgroundColor:series.key==='tokens'?'rgba(13,148,136,.06)':series.color,borderWidth:2,fill:series.key==='tokens',tension:.2,pointRadius:2}))},options:{
    responsive:true,maintainAspectRatio:false,
    interaction:{intersect:false,mode:'index'},
    onClick:(_event,elements)=>{if(daily||!elements.length)return;const point=points[elements[0]!.index];if(point){mode.value='month';month.value=Number(point.bucket.slice(5,7));changePeriod()}},
    plugins:{legend:{position:'top',labels:{color:'#819394',usePointStyle:true,boxWidth:6,font:{size:11}}},tooltip:{enabled:false,external:({chart,tooltip})=>{
      if(!tooltip.opacity||!tooltip.dataPoints.length)return
      hoveredTrend.value=points[tooltip.dataPoints[0]!.dataIndex]
      tooltipOnLeft.value=tooltip.caretX>chart.width/2
    }}},
    scales:{x:{grid:{display:false},ticks:{maxTicksLimit:7,color:'#819394',font:{size:10}}},y:{beginAtZero:true,border:{display:false},grid:{color:'rgba(128,149,147,.12)'},ticks:{maxTicksLimit:5,color:'#819394',font:{size:10},callback:value=>tokenText(BigInt(Math.max(0,Math.trunc(Number(value)))).toString())}}},
  }})
}
async function directoryChanged(state:AnalyticsDirectoryState){directory.value=state;await load()}
async function download(){
  if(!report.value||!appliedQuery.value)return
  exporting.value=true;exportAbort=new AbortController()
  const params={...appliedQuery.value,search:'',page:1,export:true,versions:JSON.stringify(report.value.versions)}
  try{await exportAnalytics(await dataAnalyticsAPI.report(params,exportAbort.signal),exportAbort.signal);if(!disposed)app.showSuccess('分析报表已导出（含趋势图和成本组成）')}
  catch(cause){if(!disposed&&!exportAbort?.signal.aborted)app.showError(cause instanceof Error?cause.message:'导出失败，请刷新报表后重试')}
  finally{exporting.value=false}
}
async function rebuild(){
  if(!rebuildStart.value||!rebuildEnd.value){app.showError('请选择补算开始和结束日期');return}
  rebuilding.value=true
  try{await dataAnalyticsAPI.rebuild(rebuildStart.value,rebuildEnd.value);app.showSuccess('补算已排队，后台分批处理');await load()}
  catch(cause){app.showError(cause instanceof Error?cause.message:'补算提交失败')}
  finally{rebuilding.value=false}
}
onMounted(()=>{void load(true);timer=setInterval(()=>{if(!document.hidden&&!loading.value&&!exporting.value&&!selectedPerson.value&&(tab.value==='org'||tab.value==='people'))void load()},30000)})
onBeforeUnmount(()=>{disposed=true;++sequence;abort?.abort();exportAbort?.abort();if(timer)clearInterval(timer);if(searchTimer)clearTimeout(searchTimer);chart?.destroy()})
</script>
