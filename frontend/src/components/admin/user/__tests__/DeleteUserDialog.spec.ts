import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import DeleteUserDialog from '../DeleteUserDialog.vue'

const searchUsers = vi.hoisted(() => vi.fn())

vi.mock('@/api/admin', () => ({
  adminAPI: { usage: { searchUsers } }
}))

vi.mock('vue-i18n', async () => ({
  ...(await vi.importActual<typeof import('vue-i18n')>('vue-i18n')),
  useI18n: () => ({ t: (key: string) => key })
}))

const user = { id: 7, email: 'intern@sub.com', username: 'Intern' }
const mountDialog = (props: { show: boolean; user: typeof user }) => mount(DeleteUserDialog, {
  props,
  global: {
    stubs: {
      BaseDialog: {
        props: ['show'],
        template: '<div v-if="show"><slot /><slot name="footer" /></div>'
      }
    }
  }
})

describe('DeleteUserDialog', () => {
  beforeEach(() => {
    searchUsers.mockReset()
  })

  it('deletes without migration by default', async () => {
    const wrapper = mountDialog({ show: true, user })

    expect(wrapper.find('[data-testid="usage-migration-target"]').exists()).toBe(false)
    await wrapper.get('[data-testid="confirm-delete-user"]').trigger('click')

    expect(wrapper.emitted('confirm')).toEqual([[undefined]])
    wrapper.unmount()
  })

  it('requires and submits an active migration target', async () => {
    vi.useFakeTimers()
    searchUsers.mockResolvedValue([
      { id: 7, email: 'intern@sub.com', username: 'Intern', deleted: false },
      { id: 8, email: 'employee@sub.com', username: 'Employee', deleted: false },
      { id: 9, email: 'left@sub.com', username: 'Left', deleted: true }
    ])
    const wrapper = mountDialog({ show: true, user })

    await wrapper.get('[data-testid="migrate-usage-checkbox"]').setValue(true)
    const confirm = wrapper.get('[data-testid="confirm-delete-user"]')
    expect(confirm.attributes('disabled')).toBeDefined()

    await wrapper.get('[data-testid="usage-migration-target"]').setValue('employee')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()

    expect(searchUsers).toHaveBeenCalledWith('employee')
    expect(wrapper.findAll('[data-testid="usage-migration-option"]')).toHaveLength(1)
    await wrapper.get('[data-testid="usage-migration-option"]').trigger('click')
    expect(confirm.attributes('disabled')).toBeUndefined()
    await confirm.trigger('click')

    expect(wrapper.emitted('confirm')).toEqual([[8]])
    wrapper.unmount()
    vi.useRealTimers()
  })

  it('clears migration state when reopened for another user', async () => {
    const wrapper = mountDialog({ show: true, user })
    await wrapper.get('[data-testid="migrate-usage-checkbox"]').setValue(true)

    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, user: { id: 10, email: 'other@sub.com', username: 'Other' } })

    expect((wrapper.get('[data-testid="migrate-usage-checkbox"]').element as HTMLInputElement).checked).toBe(false)
    expect(wrapper.find('[data-testid="usage-migration-target"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
