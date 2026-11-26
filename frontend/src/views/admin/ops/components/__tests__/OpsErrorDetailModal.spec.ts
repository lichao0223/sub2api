import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OpsErrorDetailModal from '../OpsErrorDetailModal.vue'
import { Blob as NodeBlob } from 'node:buffer'

const mocks = vi.hoisted(() => ({
  copyToClipboard: vi.fn(),
  getRequestErrorDetail: vi.fn(),
  listRequestErrorUpstreamErrors: vi.fn(),
  getErrorRequestSnapshot: vi.fn(),
  saveAs: vi.fn()
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getRequestErrorDetail: mocks.getRequestErrorDetail,
    getUpstreamErrorDetail: vi.fn(),
    getErrorRequestSnapshot: mocks.getErrorRequestSnapshot,
    listRequestErrorUpstreamErrors: mocks.listRequestErrorUpstreamErrors
  }
}))

vi.mock('file-saver', () => ({ saveAs: mocks.saveAs }))

vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError: vi.fn() })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: mocks.copyToClipboard })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

describe('OpsErrorDetailModal', () => {
  beforeEach(() => {
    mocks.getRequestErrorDetail.mockReset()
    mocks.getErrorRequestSnapshot.mockReset()
    mocks.saveAs.mockReset()
    mocks.listRequestErrorUpstreamErrors.mockReset()
    mocks.listRequestErrorUpstreamErrors.mockResolvedValue({ items: [] })
  })

  it('prioritizes upstream root cause and deduplicates diagnostic payloads', async () => {
    mocks.getRequestErrorDetail.mockResolvedValue({
      id: 1,
      created_at: '2026-08-19T00:00:00Z',
      phase: 'request',
      type: 'upstream_error',
      error_owner: 'provider',
      error_source: 'gateway',
      severity: 'P1',
      status_code: 502,
      upstream_status_code: 429,
      platform: 'openai',
      model: 'gpt-5.6',
      resolved: false,
      request_id: 'rid-1',
      message: 'All available accounts exhausted',
      error_body: '{"error":"same"}',
      upstream_error_message: 'provider rate limit exhausted',
      upstream_error_detail: '{"error":"same"}',
      upstream_errors: '[]',
      account_name: 'account',
      group_name: 'group',
      is_business_limited: false
    })

    const wrapper = shallowMount(OpsErrorDetailModal, {
      props: { show: true, errorId: 1, errorType: 'request' },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /></div>' },
          Icon: true
        }
      }
    })
    await flushPromises()

    expect(wrapper.text()).toContain('provider rate limit exhausted')
    expect(wrapper.text()).toContain('admin.ops.errorDetail.upstreamStatus')
    expect(wrapper.text()).toContain('429')
    expect(wrapper.findAll('pre')).toHaveLength(2)
    expect(wrapper.text()).not.toContain('admin.ops.errorDetail.payloads.upstream_detail')
    expect(wrapper.find('[data-testid="error-detail-reported-headers"]').exists()).toBe(false)
  })

  it('shows the identity headers that were reported upstream and copies them', async () => {
    mocks.getRequestErrorDetail.mockResolvedValue({
      id: 2,
      created_at: '2026-11-24T00:00:00Z',
      phase: 'upstream',
      error_owner: 'provider',
      status_code: 502,
      platform: 'openai',
      request_id: 'rid-2',
      upstream_errors: JSON.stringify([
        {
          account_id: 5,
          account_name: 'relay-a',
          upstream_status_code: 400,
          reported_headers: { 'x-gateway-user-id': '7' }
        }
      ])
    })

    const wrapper = shallowMount(OpsErrorDetailModal, {
      props: { show: true, errorId: 2, errorType: 'request' },
      global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, Icon: true } }
    })
    await flushPromises()

    const section = wrapper.get('[data-testid="error-detail-reported-headers"]')
    expect(section.text()).toContain('relay-a')
    expect(section.text()).toContain('x-gateway-user-id')
    await section.get('button').trigger('click')
    expect(mocks.copyToClipboard).toHaveBeenCalledWith(
      'x-gateway-user-id: 7',
      'admin.ops.errorDetail.reportedHeaders.copied'
    )
  })

  function mountSnapshotDetail(hasSnapshot = true) {
    mocks.getRequestErrorDetail.mockResolvedValue({ id: 42, phase: 'request', status_code: 400, has_request_snapshot: hasSnapshot })
    return shallowMount(OpsErrorDetailModal, {
      props: { show: true, errorId: 42, errorType: 'request' },
      global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, Icon: true } }
    })
  }

  it('previews formatted content on demand and downloads it without truncation', async () => {
    const body = JSON.stringify({ messages: [{ role: 'tool', content: 'tool output '.repeat(10000) }], tail: 'full-file-marker' }, null, 2)
    const blob = new NodeBlob([body], { type: 'application/json' })
    mocks.getErrorRequestSnapshot.mockResolvedValue(blob)
    const wrapper = mountSnapshotDetail()
    await flushPromises()
    expect(mocks.getErrorRequestSnapshot).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="view-request-snapshot"]').trigger('click')
    await flushPromises()
    expect(mocks.getErrorRequestSnapshot).toHaveBeenCalledWith(42)
    expect(wrapper.text()).toContain('admin.ops.errorDetail.requestSnapshot.previewLimited')
    expect(wrapper.text()).not.toContain('full-file-marker')
    const preview = wrapper.get('[data-testid="error-request-snapshot"] pre')
    expect(preview.element.textContent).toBe(body.slice(0, 64 * 1024))
    expect(preview.classes()).toEqual(expect.arrayContaining(['whitespace-pre-wrap', 'break-words']))
    await wrapper.get('[data-testid="download-request-snapshot"]').trigger('click')
    await flushPromises()
    expect(mocks.saveAs).toHaveBeenCalledWith(blob, 'error-42-request.json')
    expect(await blob.text()).toBe(body)
    expect(mocks.getErrorRequestSnapshot).toHaveBeenCalledTimes(1)
    await wrapper.setProps({ show: false })
    expect(wrapper.find('[data-testid="view-request-snapshot"]').exists()).toBe(false)
  })

  it('preserves formatted JSON and large integers in small previews', async () => {
    const body = '{\n  "id": 9007199254740993,\n  "tools": []\n}'
    mocks.getErrorRequestSnapshot.mockResolvedValue(new NodeBlob([body], { type: 'application/json' }))
    const wrapper = mountSnapshotDetail()
    await flushPromises()
    await wrapper.get('[data-testid="view-request-snapshot"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="error-request-snapshot"] pre').element.textContent).toBe(body)
    expect(wrapper.text()).not.toContain('admin.ops.errorDetail.requestSnapshot.previewLimited')
  })

  it('does not offer a download for old or uncaptured requests', async () => {
    const wrapper = mountSnapshotDetail(false)
    await flushPromises()
    expect(wrapper.text()).toContain('admin.ops.errorDetail.requestSnapshot.unavailable')
    expect(wrapper.find('[data-testid="download-request-snapshot"]').exists()).toBe(false)
    expect(mocks.getErrorRequestSnapshot).not.toHaveBeenCalled()
  })

  it('does not display a late snapshot after the dialog closes', async () => {
    let resolveSnapshot!: (value: NodeBlob) => void
    mocks.getErrorRequestSnapshot.mockReturnValue(new Promise(resolve => { resolveSnapshot = resolve }))
    const wrapper = mountSnapshotDetail()
    await flushPromises()
    await wrapper.get('[data-testid="view-request-snapshot"]').trigger('click')
    await wrapper.setProps({ show: false })
    resolveSnapshot(new NodeBlob(['sensitive late content']))
    await flushPromises()
    expect(wrapper.text()).not.toContain('sensitive late content')
    expect(mocks.saveAs).not.toHaveBeenCalled()
  })

  it('shows the requester alongside the account for a provider 400 in request details', async () => {
    mocks.getRequestErrorDetail.mockResolvedValue({
      id: 2,
      created_at: '2026-11-20T00:00:00Z',
      phase: 'upstream',
      type: 'upstream_error',
      error_owner: 'provider',
      status_code: 400,
      upstream_status_code: 400,
      platform: 'openai',
      account_name: 'test-provider-account',
      user_id: 42,
      user_email: 'requester@example.test'
    })

    const wrapper = shallowMount(OpsErrorDetailModal, {
      props: { show: true, errorId: 2, errorType: 'request' },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /></div>' },
          Icon: true
        }
      }
    })
    await flushPromises()

    expect(wrapper.text()).toContain('test-provider-account')
    expect(wrapper.text()).toContain('requester@example.test')
  })
})
