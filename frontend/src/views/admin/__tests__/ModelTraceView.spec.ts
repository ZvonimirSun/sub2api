import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ModelTraceView from '../ModelTraceView.vue'

const mocks = vi.hoisted(() => ({
  listMonitors: vi.fn(), getGlobalConfig: vi.fn(), updateGlobalConfig: vi.fn(),
  updateAccountConfig: vi.fn(), removeAccountMonitor: vi.fn(), listAccounts: vi.fn(), probeModel: vi.fn(),
  showError: vi.fn(), showSuccess: vi.fn(),
}))
vi.mock('@/api/admin/modeltrace', () => ({ modelTraceAPI: mocks, default: mocks }))
vi.mock('@/api/admin/accounts', () => ({ list: mocks.listAccounts, default: { list: mocks.listAccounts } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => mocks }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('@/utils/format', () => ({ formatDateTime: (value: string) => value, formatRelativeTime: (value: string) => value }))

const Dialog = defineComponent({
  props: ['show', 'title'],
  template: '<section v-if="show"><h2>{{ title }}</h2><slot /><slot name="footer" /><button @click="$emit(\'close\')">close-dialog</button></section>',
})
const Toggle = defineComponent({
  props: ['modelValue', 'disabled'],
  template: '<input type="checkbox" :checked="modelValue" :disabled="disabled" @change="$emit(\'update:modelValue\', $event.target.checked)" />',
})
const Pagination = defineComponent({
  props: ['page', 'pageSize', 'total'],
  template: '<button @click="$emit(\'update:page\', page + 1)">next-page</button>',
})

function mountView() {
  return mount(ModelTraceView, { global: { stubs: {
    AppLayout: { template: '<main><slot /></main>' }, BaseDialog: Dialog,
    Toggle, Pagination, Icon: true, PlatformTypeBadge: true,
    ConfirmDialog: { props: ['show'], template: '<button v-if="show" @click="$emit(\'confirm\')">confirm-remove</button>' },
  } } })
}
const globalConfig = { enabled: false, probe_count: 3, interval_seconds: 1, reasoning_effort: 'low', cron_expression: '*/30 * * * *' }

describe('ModelTrace explicit monitoring setup', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.listMonitors.mockResolvedValue([])
    mocks.getGlobalConfig.mockResolvedValue(globalConfig)
    mocks.updateAccountConfig.mockResolvedValue(undefined)
    mocks.removeAccountMonitor.mockResolvedValue(undefined)
    mocks.listAccounts.mockResolvedValue({ items: [{ id: 1002, name: 'Selected account', platform: 'openai', type: 'apikey' }], page: 1, total: 40 })
  })

  it('starts off, does not fetch the account pool, and avoids duplicating the layout heading', async () => {
    let resolveConfig!: (value: typeof globalConfig) => void
    mocks.getGlobalConfig.mockReturnValue(new Promise((resolve) => { resolveConfig = resolve }))
    const wrapper = mountView()
    const checkbox = wrapper.get('input[type="checkbox"]')
    expect((checkbox.element as HTMLInputElement).checked).toBe(false)
    expect(checkbox.attributes('disabled')).toBeDefined()
    resolveConfig(globalConfig)
    await flushPromises()
    expect(mocks.listAccounts).not.toHaveBeenCalled()
    expect(wrapper.find('h1').exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.modelTrace.noAccounts')
    expect(mocks.updateGlobalConfig).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('selects an account, saves only its models, and leaves the global switch off', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find((button) => button.text().includes('admin.modelTrace.addAccount'))!.trigger('click')
    await flushPromises()
    expect(mocks.listAccounts).toHaveBeenCalledWith(1, 20, expect.objectContaining({ lite: '1' }))
    await wrapper.findAll('button').find((button) => button.text().includes('Selected account'))!.trigger('click')
    await wrapper.get('input[placeholder="admin.modelTrace.addModelPlaceholder"]').setValue('gpt-5.4')
    await wrapper.findAll('button').find((button) => button.text() === 'admin.modelTrace.add')!.trigger('click')
    await wrapper.findAll('button').find((button) => button.text() === 'admin.modelTrace.saveConfig')!.trigger('click')
    await flushPromises()
    expect(mocks.updateAccountConfig).toHaveBeenCalledWith(1002, { models: ['gpt-5.4'] })
    expect(mocks.updateGlobalConfig).not.toHaveBeenCalled()
    expect(wrapper.find('select').exists()).toBe(false)
    wrapper.unmount()
  })

  it('searches and pages through candidates without adding them until configuration is saved', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find((button) => button.text().includes('admin.modelTrace.addAccount'))!.trigger('click')
    await flushPromises()
    await wrapper.get('input[type="search"]').setValue('needle')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(mocks.listAccounts).toHaveBeenLastCalledWith(1, 20, expect.objectContaining({ search: 'needle' }))
    await wrapper.findAll('button').find((button) => button.text() === 'next-page')!.trigger('click')
    await flushPromises()
    expect(mocks.listAccounts).toHaveBeenLastCalledWith(2, 20, expect.objectContaining({ search: 'needle' }))
    expect(mocks.updateAccountConfig).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('removes monitoring without deleting the upstream account', async () => {
    mocks.listMonitors.mockResolvedValue([{ account_id: 1002, account_name: 'Monitored', platform: 'openai', type: 'apikey', models: { 'gpt-5.4': {} } }])
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find((button) => button.text() === 'admin.modelTrace.removeAccount')!.trigger('click')
    await wrapper.findAll('button').find((button) => button.text() === 'confirm-remove')!.trigger('click')
    await flushPromises()
    expect(mocks.removeAccountMonitor).toHaveBeenCalledWith(1002)
    expect(wrapper.text()).toContain('admin.modelTrace.noAccounts')
    wrapper.unmount()
  })

  it('submits a background task and displays its queue state without awaiting a result', async () => {
    mocks.listMonitors.mockResolvedValue([{ account_id: 31, account_name: 'Codex', platform: 'openai', type: 'oauth', models: { 'gpt-6-sol': {} } }])
    mocks.probeModel.mockResolvedValue({ id: 'task-1', status: 'queued', created_at: '', updated_at: '' })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find((button) => button.text() === 'admin.modelTrace.runProbe')!.trigger('click')
    await flushPromises()
    expect(mocks.probeModel).toHaveBeenCalledWith(31, 'gpt-6-sol')
    expect(mocks.showSuccess).toHaveBeenCalledWith('admin.modelTrace.taskSubmitted')
    const queued = wrapper.findAll('button').find((button) => button.text() === 'admin.modelTrace.queued')!
    expect(queued.attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })
})
