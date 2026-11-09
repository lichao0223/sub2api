import type { AnalyticsReport } from '@/api/admin/dataAnalytics'
import { costManagementAPI } from '@/api/admin/costManagement'
import type { AnalyticsExportCosts } from './exportWorkbook'

export async function loadAnalyticsCosts(report: AnalyticsReport, signal?: AbortSignal): Promise<AnalyticsExportCosts[]> {
  const today = new Intl.DateTimeFormat('sv-SE', { timeZone: 'Asia/Shanghai' }).format(new Date())
  const costs: AnalyticsExportCosts[] = []
  // Fetch selected months separately so gaps in a multi-month selection aren't included.
  for (const month of report.period.months) {
    signal?.throwIfAborted()
    const start_date = `${report.period.year}-${String(month).padStart(2, '0')}-01`
    if (start_date > today) continue
    const end_date = [new Date(Date.UTC(report.period.year, month, 0)).toISOString().slice(0, 10), today].sort()[0]!
    const breakdown = await costManagementAPI.breakdown({ start_date, end_date, scope: 'total' }, signal)
    costs.push({ ...breakdown, month: start_date.slice(0, 7), start_date, end_date })
  }
  return costs
}

export async function exportAnalytics(report: AnalyticsReport, signal?: AbortSignal): Promise<void> {
  const costs = await loadAnalyticsCosts(report, signal)
  signal?.throwIfAborted()
  const worker = new Worker(new URL('./export.worker.ts', import.meta.url), { type: 'module' })
  let timeout: ReturnType<typeof setTimeout> | undefined
  let onAbort: (() => void) | undefined
  try {
    const result = await new Promise<{ buffer: ArrayBuffer; filename: string }>((resolve, reject) => {
      timeout = setTimeout(() => reject(new Error('报表生成超时，请缩小月份或部门范围')), 60000)
      onAbort = () => reject(signal?.reason || new DOMException('导出已取消', 'AbortError'))
      signal?.addEventListener('abort', onAbort, { once: true })
      worker.onmessage = event => {
        clearTimeout(timeout)
        if (event.data.error) reject(new Error(event.data.error))
        else resolve(event.data)
      }
      worker.onerror = () => { clearTimeout(timeout); reject(new Error('报表生成失败，请缩小范围后重试')) }
      worker.postMessage({ report, costs })
    })
    signal?.throwIfAborted()
    const url = URL.createObjectURL(new Blob([result.buffer], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }))
    const link = document.createElement('a'); link.href = url; link.download = result.filename
    document.body.appendChild(link); link.click(); link.remove()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  } finally {
    clearTimeout(timeout)
    if (onAbort) signal?.removeEventListener('abort', onAbort)
    worker.terminate()
  }
}
