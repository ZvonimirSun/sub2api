<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Page Header -->
      <header class="flex flex-col gap-4 border-b border-gray-200 pb-5 dark:border-dark-700 sm:flex-row sm:items-end sm:justify-between">
        <div class="min-w-0">
          <div class="flex items-center gap-2.5">
            <span class="inline-flex h-9 w-9 items-center justify-center rounded-xl bg-primary-50 text-primary-600 dark:bg-primary-950/40 dark:text-primary-400">
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M7.864 4.243A7.5 7.5 0 0119.5 10.5c0 2.92-.556 5.709-1.568 8.268M5.742 6.364A7.465 7.465 0 004.5 10.5a7.464 7.464 0 01-1.15 3.993m1.989 3.559A11.209 11.209 0 008.25 10.5a3.75 3.75 0 117.5 0c0 .527-.021 1.049-.064 1.565M12 10.5a14.94 14.94 0 01-3.6 9.75m6.633-4.596a18.666 18.666 0 01-2.485 5.346" />
              </svg>
            </span>
            <h1 class="text-xl font-bold text-gray-900 dark:text-white">
              {{ t('admin.modelTrace.title') }}
            </h1>
          </div>
          <p class="mt-1.5 max-w-3xl text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.modelTrace.description') }}
          </p>
          <div class="mt-3 flex flex-wrap gap-2 text-xs text-gray-600 dark:text-gray-300">
            <span class="rounded bg-gray-100 px-2 py-1 dark:bg-dark-700">ModelTrace 统计指纹 (3次采样·low思考)</span>
            <span class="rounded bg-gray-100 px-2 py-1 dark:bg-dark-700">单账号内严格串行保护并发</span>
            <span class="rounded bg-gray-100 px-2 py-1 dark:bg-dark-700">复用统一账号测试链路 · 无侵入</span>
            <span class="rounded bg-gray-100 px-2 py-1 dark:bg-dark-700">仅客观数据记录 · 无自动化封禁</span>
          </div>
        </div>

        <!-- Header Actions: Master Switch + Global Settings + Refresh -->
        <div class="flex flex-wrap items-center gap-3">
          <!-- Master Switch -->
          <div class="flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 py-1.5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <span class="text-xs font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.modelTrace.globalSwitch') }}:
            </span>
            <Toggle
              :model-value="globalConfig.enabled"
              :disabled="updatingGlobalConfig"
              @update:model-value="toggleGlobalSwitch"
            />
            <span
              class="text-[11px] font-semibold"
              :class="globalConfig.enabled ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-400'"
            >
              {{ globalConfig.enabled ? t('admin.modelTrace.globalSwitchEnabled') : t('admin.modelTrace.globalSwitchDisabled') }}
            </span>
          </div>

          <!-- Global Settings Button -->
          <button
            type="button"
            class="btn btn-secondary"
            :title="t('admin.modelTrace.globalSettings')"
            @click="openGlobalConfigDialog"
          >
            <Icon name="cog" size="sm" />
            <span>{{ t('admin.modelTrace.globalSettings') }}</span>
          </button>

          <!-- Refresh Button -->
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="loading"
            :title="t('admin.modelTrace.refresh')"
            @click="loadAll"
          >
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
            <span>{{ t('admin.modelTrace.refresh') }}</span>
          </button>
        </div>
      </header>

      <!-- Stat Cards -->
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <div class="card flex items-center gap-4 p-4 dark:bg-dark-800">
          <div class="flex h-12 w-12 items-center justify-center rounded-xl bg-blue-50 text-blue-600 dark:bg-blue-950/40 dark:text-blue-400">
            <Icon name="server" size="md" />
          </div>
          <div>
            <div class="text-xs font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.modelTrace.totalAccounts') }}
            </div>
            <div class="mt-0.5 text-xl font-bold text-gray-900 dark:text-white">
              {{ enabledAccountsCount }} / {{ monitors.length }}
            </div>
          </div>
        </div>

        <div class="card flex items-center gap-4 p-4 dark:bg-dark-800">
          <div class="flex h-12 w-12 items-center justify-center rounded-xl bg-emerald-50 text-emerald-600 dark:bg-emerald-950/40 dark:text-emerald-400">
            <Icon name="sparkles" size="md" />
          </div>
          <div>
            <div class="text-xs font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.modelTrace.totalModels') }}
            </div>
            <div class="mt-0.5 text-xl font-bold text-gray-900 dark:text-white">
              {{ totalConfiguredModels }}
            </div>
          </div>
        </div>

        <div class="card flex items-center gap-4 p-4 dark:bg-dark-800">
          <div class="flex h-12 w-12 items-center justify-center rounded-xl bg-purple-50 text-purple-600 dark:bg-purple-950/40 dark:text-purple-400">
            <Icon name="clock" size="md" />
          </div>
          <div>
            <div class="text-xs font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.modelTrace.lastCycleRun') }}
            </div>
            <div class="mt-0.5 text-sm font-semibold text-gray-900 dark:text-white">
              {{ lastCycleRunTimeFormatted }}
            </div>
          </div>
        </div>
      </div>

      <!-- Loading State -->
      <div v-if="loading && monitors.length === 0" class="flex min-h-48 items-center justify-center text-sm text-gray-500">
        <Icon name="refresh" size="md" class="mr-2 animate-spin text-primary-500" />
        {{ t('common.loading') }}
      </div>

      <!-- Empty State -->
      <div
        v-else-if="monitors.length === 0"
        class="card flex min-h-56 flex-col items-center justify-center border-dashed border-gray-300 p-8 text-center dark:border-dark-600 dark:bg-dark-800"
      >
        <Icon name="cube" size="xl" class="text-gray-400" />
        <p class="mt-3 font-medium text-gray-800 dark:text-gray-200">
          {{ t('admin.modelTrace.noAccounts') }}
        </p>
      </div>

      <!-- Accounts List -->
      <div v-else class="space-y-5">
        <div
          v-for="account in monitors"
          :key="account.account_id"
          class="card overflow-hidden border border-gray-200 dark:border-dark-700 dark:bg-dark-800"
        >
          <!-- Account Card Header -->
          <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 bg-gray-50/60 p-4 dark:border-dark-700 dark:bg-dark-850">
            <div class="flex flex-wrap items-center gap-3">
              <!-- Quick Account Toggle Switch -->
              <Toggle
                :model-value="account.enabled"
                :disabled="togglingMap[account.account_id]"
                :title="account.enabled ? '点击暂停此账号的定时监控' : '点击开启此账号的定时监控'"
                @update:model-value="(val) => handleToggleAccount(account, val)"
              />

              <span class="text-base font-semibold text-gray-900 dark:text-white">
                {{ account.account_name }}
              </span>
              <PlatformTypeBadge :platform="account.platform" :type="account.type" />

              <!-- Cron & Enabled Badge -->
              <span
                v-if="account.enabled"
                class="inline-flex items-center gap-1 rounded-md bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 ring-1 ring-inset ring-emerald-600/20 dark:bg-emerald-900/30 dark:text-emerald-300"
              >
                <span class="h-1.5 w-1.5 rounded-full bg-emerald-500"></span>
                定时中 ({{ account.cron || '继承全局' }})
              </span>
              <span
                v-else
                class="inline-flex items-center gap-1 rounded-md bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-400"
              >
                {{ t('admin.modelTrace.accountProbingPaused') }}
              </span>

              <!-- Last run time -->
              <span v-if="account.last_run_at" class="text-xs text-gray-400 dark:text-gray-500">
                最近周期: {{ formatRelativeTime(account.last_run_at) }}
              </span>
            </div>

            <!-- Action buttons -->
            <div class="flex items-center gap-2">
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                @click="openConfigDialog(account)"
              >
                <Icon name="cog" size="sm" class="mr-1" />
                {{ t('admin.modelTrace.configureModels') }}
              </button>
            </div>
          </div>

          <!-- Account Card Body: Models Table -->
          <div class="p-0">
            <div v-if="Object.keys(account.models).length === 0" class="p-6 text-center text-sm text-gray-500 dark:text-gray-400">
              <p>{{ t('admin.modelTrace.noModelsConfigured') }}</p>
              <button
                type="button"
                class="btn btn-secondary btn-sm mt-3"
                @click="openConfigDialog(account)"
              >
                <Icon name="plus" size="sm" class="mr-1" />
                {{ t('admin.modelTrace.configureModels') }}
              </button>
            </div>

            <div v-else class="overflow-x-auto">
              <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
                <thead class="bg-gray-50 dark:bg-dark-850">
                  <tr>
                    <th scope="col" class="px-4 py-2.5 text-left text-xs font-semibold text-gray-600 dark:text-gray-300">
                      {{ t('admin.modelTrace.configuredModel') }}
                    </th>
                    <th scope="col" class="px-4 py-2.5 text-left text-xs font-semibold text-gray-600 dark:text-gray-300">
                      {{ t('admin.modelTrace.detectedModel') }}
                    </th>
                    <th scope="col" class="px-4 py-2.5 text-left text-xs font-semibold text-gray-600 dark:text-gray-300">
                      {{ t('admin.modelTrace.probability') }}
                    </th>
                    <th scope="col" class="px-4 py-2.5 text-left text-xs font-semibold text-gray-600 dark:text-gray-300">
                      {{ t('admin.modelTrace.latency') }}
                    </th>
                    <th scope="col" class="px-4 py-2.5 text-left text-xs font-semibold text-gray-600 dark:text-gray-300">
                      {{ t('admin.modelTrace.testedAt') }}
                    </th>
                    <th scope="col" class="px-4 py-2.5 text-right text-xs font-semibold text-gray-600 dark:text-gray-300">
                      {{ t('admin.modelTrace.actions') }}
                    </th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-800">
                  <tr
                    v-for="(cfg, modelId) in account.models"
                    :key="modelId"
                    class="transition-colors hover:bg-gray-50/50 dark:hover:bg-dark-750/50"
                  >
                    <!-- Configured Model -->
                    <td class="whitespace-nowrap px-4 py-3 font-mono text-sm font-semibold text-gray-900 dark:text-gray-100">
                      {{ modelId }}
                    </td>

                    <!-- Detected Model -->
                    <td class="whitespace-nowrap px-4 py-3 text-sm">
                      <template v-if="cfg.latest">
                        <span
                          v-if="cfg.latest.error"
                          class="inline-flex items-center rounded-md bg-rose-50 px-2 py-0.5 font-mono text-xs font-medium text-rose-700 ring-1 ring-inset ring-rose-600/20 dark:bg-rose-900/30 dark:text-rose-300"
                          :title="cfg.latest.error"
                        >
                          探测异常: {{ cfg.latest.error }}
                        </span>
                        <span
                          v-else
                          class="inline-flex items-center rounded-md bg-blue-50 px-2.5 py-0.5 font-mono text-xs font-semibold text-blue-700 ring-1 ring-inset ring-blue-700/10 dark:bg-blue-900/30 dark:text-blue-300"
                        >
                          {{ cfg.latest.detected_model }}
                        </span>
                      </template>
                      <span v-else class="text-xs text-gray-400 dark:text-gray-500">
                        {{ t('admin.modelTrace.untested') }}
                      </span>
                    </td>

                    <!-- Probability -->
                    <td class="whitespace-nowrap px-4 py-3 text-sm">
                      <template v-if="cfg.latest && !cfg.latest.error">
                        <div class="flex items-center gap-2">
                          <span
                            class="font-mono text-xs font-semibold"
                            :class="probTextClass(cfg.latest.probability)"
                          >
                            {{ (cfg.latest.probability * 100).toFixed(1) }}%
                          </span>
                          <div class="h-1.5 w-16 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                            <div
                              class="h-full rounded-full transition-all duration-300"
                              :class="probBarClass(cfg.latest.probability)"
                              :style="{ width: `${Math.min(100, Math.max(0, cfg.latest.probability * 100))}%` }"
                            ></div>
                          </div>
                        </div>
                      </template>
                      <span v-else class="text-xs text-gray-400">-</span>
                    </td>

                    <!-- Latency -->
                    <td class="whitespace-nowrap px-4 py-3 font-mono text-xs text-gray-600 dark:text-gray-300">
                      {{ cfg.latest && !cfg.latest.error ? `${cfg.latest.latency_ms} ms` : '-' }}
                    </td>

                    <!-- Tested At -->
                    <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-500 dark:text-gray-400">
                      <span v-if="cfg.latest" :title="formatDateTime(cfg.latest.tested_at)">
                        {{ formatRelativeTime(cfg.latest.tested_at) }}
                      </span>
                      <span v-else>-</span>
                    </td>

                    <!-- Actions -->
                    <td class="whitespace-nowrap px-4 py-3 text-right text-xs">
                      <div class="inline-flex items-center gap-1.5">
                        <button
                          type="button"
                          class="btn btn-secondary btn-sm"
                          :disabled="isProbing(account.account_id, String(modelId))"
                          @click="handleRunProbe(account, String(modelId))"
                        >
                          <Icon
                            name="refresh"
                            size="xs"
                            class="mr-1"
                            :class="{ 'animate-spin': isProbing(account.account_id, String(modelId)) }"
                          />
                          <span>{{ isProbing(account.account_id, String(modelId)) ? t('admin.modelTrace.probing') : t('admin.modelTrace.runProbe') }}</span>
                        </button>

                        <button
                          type="button"
                          class="btn btn-secondary btn-sm"
                          :disabled="!cfg.history || cfg.history.length === 0"
                          @click="openHistoryDialog(account, String(modelId), cfg.history || [])"
                        >
                          <Icon name="clock" size="xs" class="mr-1" />
                          <span>{{ t('admin.modelTrace.history') }}</span>
                          <span v-if="cfg.history && cfg.history.length > 0" class="ml-0.5 text-[10px] text-gray-400">
                            ({{ cfg.history.length }})
                          </span>
                        </button>
                      </div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Configure Models Dialog (Per-Account) -->
    <BaseDialog
      :show="showConfigDialog"
      :title="t('admin.modelTrace.modalTitle', { name: editingAccount?.account_name || '' })"
      @close="closeConfigDialog"
    >
      <form @submit.prevent="saveConfig" class="space-y-5">
        <!-- Enable scheduled probing switch -->
        <div class="flex items-center justify-between">
          <div>
            <label class="text-sm font-medium text-gray-900 dark:text-white">
              {{ t('admin.modelTrace.enabledSchedule') }}
            </label>
            <p class="text-xs text-gray-500 dark:text-gray-400">
              开启后由后台定时轮询任务按设定周期自动串行测试
            </p>
          </div>
          <Toggle v-model="configForm.enabled" />
        </div>

        <!-- Cron Expression -->
        <div>
          <label class="block text-sm font-medium text-gray-900 dark:text-white">
            {{ t('admin.modelTrace.cronSchedule') }}
          </label>
          <div class="mt-1">
            <input
              v-model.trim="configForm.cron"
              type="text"
              class="input font-mono"
              :placeholder="t('admin.modelTrace.cronPlaceholder')"
            />
          </div>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.modelTrace.cronHelp') }}
          </p>

          <!-- Preset buttons -->
          <div class="mt-2 flex flex-wrap gap-1.5">
            <button
              type="button"
              class="rounded border border-gray-200 px-2 py-0.5 text-xs text-gray-600 hover:bg-gray-100 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
              @click="configForm.cron = '*/15 * * * *'"
            >
              {{ t('admin.modelTrace.every15Min') }}
            </button>
            <button
              type="button"
              class="rounded border border-gray-200 px-2 py-0.5 text-xs text-gray-600 hover:bg-gray-100 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
              @click="configForm.cron = '*/30 * * * *'"
            >
              {{ t('admin.modelTrace.every30Min') }}
            </button>
            <button
              type="button"
              class="rounded border border-gray-200 px-2 py-0.5 text-xs text-gray-600 hover:bg-gray-100 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
              @click="configForm.cron = '0 * * * *'"
            >
              {{ t('admin.modelTrace.every1Hour') }}
            </button>
            <button
              type="button"
              class="rounded border border-gray-200 px-2 py-0.5 text-xs text-gray-600 hover:bg-gray-100 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
              @click="configForm.cron = '0 */2 * * *'"
            >
              {{ t('admin.modelTrace.every2Hours') }}
            </button>
          </div>
        </div>

        <!-- Models List -->
        <div>
          <div class="flex items-center justify-between">
            <label class="block text-sm font-medium text-gray-900 dark:text-white">
              {{ t('admin.modelTrace.monitoredModels') }}
            </label>
            <button
              type="button"
              class="text-xs text-primary-600 hover:underline dark:text-primary-400"
              @click="showBatchInput = !showBatchInput"
            >
              {{ showBatchInput ? '收起批量粘贴' : '批量粘贴导入' }}
            </button>
          </div>

          <!-- Batch Textarea -->
          <div v-if="showBatchInput" class="mt-2 space-y-2">
            <textarea
              v-model="batchInputText"
              rows="3"
              class="input font-mono text-xs"
              :placeholder="t('admin.modelTrace.batchAddPlaceholder')"
            ></textarea>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              @click="handleBatchAddModels"
            >
              {{ t('admin.modelTrace.batchAdd') }}
            </button>
          </div>

          <!-- Single Add Input -->
          <div class="mt-2 flex gap-2">
            <input
              v-model.trim="newModelInput"
              type="text"
              class="input flex-1 font-mono text-sm"
              :placeholder="t('admin.modelTrace.addModelPlaceholder')"
              @keydown.enter.prevent="handleAddModel"
            />
            <button
              type="button"
              class="btn btn-secondary"
              @click="handleAddModel"
            >
              {{ t('admin.modelTrace.add') }}
            </button>
          </div>

          <!-- Model Tags -->
          <div class="mt-3 flex flex-wrap gap-2">
            <span
              v-for="(m, idx) in configForm.models"
              :key="m"
              class="inline-flex items-center gap-1.5 rounded-lg bg-gray-100 px-2.5 py-1 font-mono text-xs text-gray-800 dark:bg-dark-700 dark:text-gray-200"
            >
              <span>{{ m }}</span>
              <button
                type="button"
                class="rounded-full text-gray-400 hover:bg-gray-200 hover:text-gray-600 dark:hover:bg-dark-600 dark:hover:text-gray-200"
                @click="configForm.models.splice(idx, 1)"
              >
                <Icon name="x" size="xs" />
              </button>
            </span>
            <span v-if="configForm.models.length === 0" class="text-xs text-gray-400">
              尚未添加任何模型
            </span>
          </div>
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-2">
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="saving"
            @click="closeConfigDialog"
          >
            {{ t('common.cancel') }}
          </button>
          <button
            type="button"
            class="btn btn-primary"
            :disabled="saving"
            @click="saveConfig"
          >
            {{ saving ? t('common.submitting') : t('admin.modelTrace.saveConfig') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Global Settings Dialog -->
    <BaseDialog
      :show="showGlobalConfigDialog"
      :title="t('admin.modelTrace.globalSettingsTitle')"
      @close="showGlobalConfigDialog = false"
    >
      <form @submit.prevent="saveGlobalConfig" class="space-y-4">
        <!-- Master Switch in Settings -->
        <div class="flex items-center justify-between border-b border-gray-100 pb-3 dark:border-dark-700">
          <div>
            <label class="text-sm font-medium text-gray-900 dark:text-white">
              {{ t('admin.modelTrace.globalSwitch') }}
            </label>
            <p class="text-xs text-gray-500 dark:text-gray-400">
              关闭后将暂停整个系统所有账号的后台周期性轮询探测
            </p>
          </div>
          <Toggle v-model="globalForm.enabled" />
        </div>

        <!-- Probe Count -->
        <div>
          <label class="block text-sm font-medium text-gray-900 dark:text-white">
            {{ t('admin.modelTrace.probeCount') }}
          </label>
          <input
            v-model.number="globalForm.probe_count"
            type="number"
            min="1"
            max="10"
            class="input mt-1 font-mono"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.modelTrace.probeCountHelp') }}
          </p>
        </div>

        <!-- Interval Seconds -->
        <div>
          <label class="block text-sm font-medium text-gray-900 dark:text-white">
            {{ t('admin.modelTrace.intervalSeconds') }}
          </label>
          <input
            v-model.number="globalForm.interval_seconds"
            type="number"
            min="0"
            max="60"
            class="input mt-1 font-mono"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.modelTrace.intervalSecondsHelp') }}
          </p>
        </div>

        <!-- Reasoning Effort -->
        <div>
          <label class="block text-sm font-medium text-gray-900 dark:text-white">
            {{ t('admin.modelTrace.reasoningEffort') }}
          </label>
          <select v-model="globalForm.reasoning_effort" class="input mt-1">
            <option value="low">low (推荐，低耗时低消耗)</option>
            <option value="medium">medium</option>
            <option value="high">high</option>
          </select>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.modelTrace.reasoningEffortHelp') }}
          </p>
        </div>

        <!-- Default Cron Expression -->
        <div>
          <label class="block text-sm font-medium text-gray-900 dark:text-white">
            {{ t('admin.modelTrace.cronExpression') }}
          </label>
          <input
            v-model.trim="globalForm.cron_expression"
            type="text"
            class="input mt-1 font-mono"
            placeholder="*/30 * * * *"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.modelTrace.cronHelp') }}
          </p>
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-2">
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="savingGlobalConfig"
            @click="showGlobalConfigDialog = false"
          >
            {{ t('common.cancel') }}
          </button>
          <button
            type="button"
            class="btn btn-primary"
            :disabled="savingGlobalConfig"
            @click="saveGlobalConfig"
          >
            {{ savingGlobalConfig ? t('common.submitting') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- History Dialog -->
    <BaseDialog
      :show="showHistoryDialog"
      :title="t('admin.modelTrace.historyTitle', { name: historyAccount?.account_name || '', model: historyModelId })"
      @close="showHistoryDialog = false"
    >
      <div v-if="historyRecords.length === 0" class="py-8 text-center text-sm text-gray-500">
        {{ t('admin.modelTrace.noHistory') }}
      </div>

      <div v-else class="overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
          <thead class="bg-gray-50 dark:bg-dark-850">
            <tr>
              <th scope="col" class="px-3 py-2 text-left text-xs font-semibold text-gray-600 dark:text-gray-300">
                {{ t('admin.modelTrace.testedAt') }}
              </th>
              <th scope="col" class="px-3 py-2 text-left text-xs font-semibold text-gray-600 dark:text-gray-300">
                {{ t('admin.modelTrace.detectedModel') }}
              </th>
              <th scope="col" class="px-3 py-2 text-left text-xs font-semibold text-gray-600 dark:text-gray-300">
                {{ t('admin.modelTrace.probability') }}
              </th>
              <th scope="col" class="px-3 py-2 text-left text-xs font-semibold text-gray-600 dark:text-gray-300">
                {{ t('admin.modelTrace.latency') }}
              </th>
              <th scope="col" class="px-3 py-2 text-left text-xs font-semibold text-gray-600 dark:text-gray-300">
                {{ t('admin.modelTrace.status') }}
              </th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-800">
            <tr v-for="(rec, idx) in historyRecords" :key="idx" class="text-xs">
              <td class="whitespace-nowrap px-3 py-2 text-gray-500 dark:text-gray-400">
                {{ formatDateTime(rec.tested_at) }}
              </td>
              <td class="whitespace-nowrap px-3 py-2 font-mono font-medium text-gray-900 dark:text-gray-100">
                {{ rec.detected_model || '-' }}
              </td>
              <td class="whitespace-nowrap px-3 py-2">
                <span v-if="!rec.error" :class="probTextClass(rec.probability)" class="font-mono font-semibold">
                  {{ (rec.probability * 100).toFixed(1) }}%
                </span>
                <span v-else class="text-gray-400">-</span>
              </td>
              <td class="whitespace-nowrap px-3 py-2 font-mono text-gray-600 dark:text-gray-300">
                {{ rec.latency_ms ? `${rec.latency_ms} ms` : '-' }}
              </td>
              <td class="px-3 py-2">
                <span
                  v-if="rec.error"
                  class="inline-flex items-center rounded bg-rose-50 px-2 py-0.5 text-[11px] font-medium text-rose-700 ring-1 ring-inset ring-rose-600/20 dark:bg-rose-900/30 dark:text-rose-300"
                  :title="rec.error"
                >
                  {{ rec.error }}
                </span>
                <span
                  v-else
                  class="inline-flex items-center rounded bg-emerald-50 px-2 py-0.5 text-[11px] font-medium text-emerald-700 ring-1 ring-inset ring-emerald-600/20 dark:bg-emerald-900/30 dark:text-emerald-300"
                >
                  {{ t('admin.modelTrace.success') }}
                </span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <template #footer>
        <div class="flex justify-end">
          <button type="button" class="btn btn-secondary" @click="showHistoryDialog = false">
            {{ t('common.close') }}
          </button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import {
  modelTraceAPI,
  type ModelTraceMonitorItem,
  type ModelTraceRecord,
  type ModelTraceGlobalConfig
} from '@/api/admin/modeltrace'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime, formatRelativeTime } from '@/utils/format'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import PlatformTypeBadge from '@/components/common/PlatformTypeBadge.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const monitors = ref<ModelTraceMonitorItem[]>([])
const probingMap = reactive<Record<string, boolean>>({})
const togglingMap = reactive<Record<number, boolean>>({})

// Global Config State
const globalConfig = ref<ModelTraceGlobalConfig>({
  enabled: true,
  probe_count: 3,
  interval_seconds: 1,
  reasoning_effort: 'low',
  cron_expression: '*/30 * * * *'
})
const updatingGlobalConfig = ref(false)
const showGlobalConfigDialog = ref(false)
const savingGlobalConfig = ref(false)
const globalForm = reactive<ModelTraceGlobalConfig>({
  enabled: true,
  probe_count: 3,
  interval_seconds: 1,
  reasoning_effort: 'low',
  cron_expression: '*/30 * * * *'
})

// Stats
const enabledAccountsCount = computed(() => {
  return monitors.value.filter((m) => m.enabled).length
})

const totalConfiguredModels = computed(() => {
  return monitors.value.reduce((acc, m) => acc + Object.keys(m.models).length, 0)
})

const lastCycleRunTimeFormatted = computed(() => {
  let latestTime: string | null = null
  for (const m of monitors.value) {
    if (m.last_run_at) {
      if (!latestTime || new Date(m.last_run_at) > new Date(latestTime)) {
        latestTime = m.last_run_at
      }
    }
  }
  return latestTime ? formatRelativeTime(latestTime) : t('admin.modelTrace.untested')
})

function probTextClass(prob: number): string {
  if (prob >= 0.8) return 'text-emerald-600 dark:text-emerald-400'
  if (prob >= 0.5) return 'text-amber-600 dark:text-amber-400'
  return 'text-gray-500 dark:text-gray-400'
}

function probBarClass(prob: number): string {
  if (prob >= 0.8) return 'bg-emerald-500'
  if (prob >= 0.5) return 'bg-amber-500'
  return 'bg-gray-400'
}

function isProbing(accountId: number, modelId: string): boolean {
  return Boolean(probingMap[`${accountId}:${modelId}`])
}

async function loadAll() {
  loading.value = true
  try {
    const [monitorsData, cfgData] = await Promise.all([
      modelTraceAPI.listMonitors(),
      modelTraceAPI.getGlobalConfig()
    ])
    monitors.value = monitorsData
    globalConfig.value = cfgData
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

// Master Toggle
async function toggleGlobalSwitch(val: boolean) {
  updatingGlobalConfig.value = true
  const oldVal = globalConfig.value.enabled
  globalConfig.value.enabled = val

  try {
    await modelTraceAPI.updateGlobalConfig({
      ...globalConfig.value,
      enabled: val
    })
    appStore.showSuccess(val ? t('admin.modelTrace.globalSwitchEnabled') : t('admin.modelTrace.globalSwitchDisabled'))
  } catch (err: unknown) {
    globalConfig.value.enabled = oldVal
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    updatingGlobalConfig.value = false
  }
}

// Per-Account Quick Toggle
async function handleToggleAccount(account: ModelTraceMonitorItem, enabled: boolean) {
  togglingMap[account.account_id] = true
  const oldVal = account.enabled
  account.enabled = enabled

  try {
    await modelTraceAPI.toggleAccountEnabled(account.account_id, enabled)
    appStore.showSuccess(t('admin.modelTrace.toggleAccountSuccess'))
  } catch (err: unknown) {
    account.enabled = oldVal
    appStore.showError(extractApiErrorMessage(err, t('admin.modelTrace.toggleAccountFailed')))
  } finally {
    togglingMap[account.account_id] = false
  }
}

// Global Config Dialog
function openGlobalConfigDialog() {
  globalForm.enabled = globalConfig.value.enabled
  globalForm.probe_count = globalConfig.value.probe_count
  globalForm.interval_seconds = globalConfig.value.interval_seconds
  globalForm.reasoning_effort = globalConfig.value.reasoning_effort
  globalForm.cron_expression = globalConfig.value.cron_expression
  showGlobalConfigDialog.value = true
}

async function saveGlobalConfig() {
  savingGlobalConfig.value = true
  try {
    const res = await modelTraceAPI.updateGlobalConfig({ ...globalForm })
    if (res.config) {
      globalConfig.value = res.config
    }
    appStore.showSuccess(t('admin.modelTrace.configSaved'))
    showGlobalConfigDialog.value = false
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.modelTrace.configSaveFailed')))
  } finally {
    savingGlobalConfig.value = false
  }
}

// Single Model Probe
async function handleRunProbe(account: ModelTraceMonitorItem, modelId: string) {
  const key = `${account.account_id}:${modelId}`
  if (probingMap[key]) return
  probingMap[key] = true

  try {
    const record = await modelTraceAPI.probeModel(account.account_id, modelId)
    if (!account.models[modelId]) {
      account.models[modelId] = {}
    }
    const previous = account.models[modelId].latest
    account.models[modelId].latest = record
    if (!account.models[modelId].history) {
      account.models[modelId].history = []
    }
    if (previous) account.models[modelId].history!.unshift(previous)
    if (account.models[modelId].history!.length > 5) {
      account.models[modelId].history = account.models[modelId].history!.slice(0, 5)
    }
    account.last_run_at = record.tested_at

    if (record.error) {
      appStore.showError(t('admin.modelTrace.probeFailed', { error: record.error }))
    } else {
      appStore.showSuccess(
        t('admin.modelTrace.probeSuccess', {
          model: record.detected_model,
          prob: (record.probability * 100).toFixed(1)
        })
      )
    }
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    probingMap[key] = false
  }
}

// Config Dialog (Account)
const showConfigDialog = ref(false)
const editingAccount = ref<ModelTraceMonitorItem | null>(null)
const configForm = reactive({
  enabled: true,
  cron: '',
  models: [] as string[]
})
const newModelInput = ref('')
const showBatchInput = ref(false)
const batchInputText = ref('')
const saving = ref(false)

function openConfigDialog(account: ModelTraceMonitorItem) {
  editingAccount.value = account
  configForm.enabled = account.enabled
  configForm.cron = account.cron || ''
  configForm.models = Object.keys(account.models)
  newModelInput.value = ''
  showBatchInput.value = false
  batchInputText.value = ''
  showConfigDialog.value = true
}

function closeConfigDialog() {
  showConfigDialog.value = false
  editingAccount.value = null
}

function handleAddModel() {
  const m = newModelInput.value.trim()
  if (!m) return
  if (!configForm.models.includes(m)) {
    configForm.models.push(m)
  }
  newModelInput.value = ''
}

function handleBatchAddModels() {
  if (!batchInputText.value.trim()) return
  const parts = batchInputText.value
    .split(/[\n,]+/)
    .map((s) => s.trim())
    .filter(Boolean)

  for (const part of parts) {
    if (!configForm.models.includes(part)) {
      configForm.models.push(part)
    }
  }
  batchInputText.value = ''
  showBatchInput.value = false
}

async function saveConfig() {
  if (!editingAccount.value) return
  saving.value = true

  try {
    await modelTraceAPI.updateAccountConfig(editingAccount.value.account_id, {
      enabled: configForm.enabled,
      cron_expression: configForm.cron.trim(),
      models: configForm.models
    })
    appStore.showSuccess(t('admin.modelTrace.configSaved'))
    showConfigDialog.value = false
    await loadAll()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.modelTrace.configSaveFailed')))
  } finally {
    saving.value = false
  }
}

// History Dialog
const showHistoryDialog = ref(false)
const historyAccount = ref<ModelTraceMonitorItem | null>(null)
const historyModelId = ref('')
const historyRecords = ref<ModelTraceRecord[]>([])

function openHistoryDialog(
  account: ModelTraceMonitorItem,
  modelId: string,
  history: ModelTraceRecord[]
) {
  historyAccount.value = account
  historyModelId.value = modelId
  historyRecords.value = history
  showHistoryDialog.value = true
}

onMounted(() => {
  loadAll()
})
</script>
