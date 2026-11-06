import type { AnalyticsReport } from '@/api/admin/dataAnalytics'

export async function exportAnalytics(report: AnalyticsReport): Promise<void> {
  const worker = new Worker(new URL('./export.worker.ts', import.meta.url), { type: 'module' })
  try {
    const result = await new Promise<{ buffer: ArrayBuffer; filename: string }>((resolve, reject) => {
      const timeout = setTimeout(() => reject(new Error('报表生成超时，请缩小月份或部门范围')), 60000)
      worker.onmessage = event => {
        clearTimeout(timeout)
        if (event.data.error) reject(new Error(event.data.error))
        else resolve(event.data)
      }
      worker.onerror = () => { clearTimeout(timeout); reject(new Error('报表生成失败，请缩小范围后重试')) }
      worker.postMessage(report)
    })
    const url = URL.createObjectURL(new Blob([result.buffer], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }))
    const link = document.createElement('a'); link.href = url; link.download = result.filename
    document.body.appendChild(link); link.click(); link.remove()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  } finally { worker.terminate() }
}
