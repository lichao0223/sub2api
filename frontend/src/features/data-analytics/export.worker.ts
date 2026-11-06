import { buildAnalyticsWorkbook } from './exportWorkbook'
import type { AnalyticsReport } from '@/api/admin/dataAnalytics'

const worker = globalThis as unknown as {
  onmessage: (event: MessageEvent<AnalyticsReport>) => void
  postMessage: (data: unknown, transfer?: Transferable[]) => void
}
worker.onmessage = async event => {
  try {
    const result = await buildAnalyticsWorkbook(event.data)
    worker.postMessage(result, [result.buffer])
  } catch (error) { worker.postMessage({ error: error instanceof Error ? error.message : '生成报表失败' }) }
}
