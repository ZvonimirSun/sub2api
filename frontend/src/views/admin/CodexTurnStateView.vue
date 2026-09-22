<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Header Actions -->
      <div class="flex flex-wrap items-center justify-end gap-2">
        <!-- Auto-Refresh Toggle -->
        <button
          type="button"
          class="btn btn-sm"
          :class="autoRefresh ? 'btn-primary' : 'btn-secondary'"
          @click="toggleAutoRefresh"
          :title="t('admin.codexTurnState.filters.autoRefresh')"
        >
          <Icon name="sync" size="xs" :class="{ 'animate-spin': autoRefresh }" />
          <span>{{ t('admin.codexTurnState.filters.autoRefresh') }}</span>
        </button>

        <!-- Refresh -->
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="loading"
          @click="loadData"
          :title="t('admin.codexTurnState.actions.refresh')"
        >
          <Icon name="refresh" size="xs" :class="{ 'animate-spin': loading }" />
          <span>{{ t('admin.codexTurnState.actions.refresh') }}</span>
        </button>

        <!-- Manage Proxies -->
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          @click="openProxiesModal"
        >
          <Icon name="server" size="xs" />
          <span>{{ t('admin.codexTurnState.actions.manageProxies') }}</span>
        </button>

        <!-- Add Monitored Model -->
        <button
          type="button"
          class="btn btn-primary btn-sm"
          @click="openAddModal"
        >
          <Icon name="plus" size="xs" />
          <span>{{ t('admin.codexTurnState.actions.addModel') }}</span>
        </button>
      </div>

      <!-- No Proxy Configured Warning Banner -->
      <div
        v-if="hasNoProxies"
        class="rounded-xl border border-amber-300 bg-amber-50 p-4 shadow-sm dark:border-amber-800/60 dark:bg-amber-950/40"
      >
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div class="flex items-start gap-3">
            <div class="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-amber-100 text-amber-600 dark:bg-amber-900/50 dark:text-amber-400">
              <Icon name="exclamationTriangle" size="sm" />
            </div>
            <div>
              <h3 class="text-sm font-semibold text-amber-900 dark:text-amber-200">
                {{ t('admin.codexTurnState.noProxyWarning.title') }}
              </h3>
              <p class="mt-0.5 text-xs text-amber-700 dark:text-amber-300/90">
                {{ t('admin.codexTurnState.noProxyWarning.description') }}
              </p>
            </div>
          </div>
          <button
            type="button"
            class="btn btn-warning btn-sm shrink-0 self-start sm:self-center"
            @click="openProxiesModal"
          >
            <Icon name="server" size="xs" />
            <span>{{ t('admin.codexTurnState.noProxyWarning.action') }}</span>
          </button>
        </div>
      </div>

      <!-- Overview Stats Banner -->
      <div class="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
        <!-- Total Accounts -->
        <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="flex items-center gap-3">
            <div class="flex h-10 w-10 items-center justify-center rounded-lg bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400">
              <Icon name="globe" size="md" />
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('admin.codexTurnState.overview.totalAccounts') }}
              </p>
              <p class="text-xl font-semibold text-gray-900 dark:text-white">
                {{ overview?.total_accounts ?? 0 }}
              </p>
            </div>
          </div>
        </div>

        <!-- Total Models -->
        <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="flex items-center gap-3">
            <div class="flex h-10 w-10 items-center justify-center rounded-lg bg-indigo-50 text-indigo-600 dark:bg-indigo-900/30 dark:text-indigo-400">
              <Icon name="cube" size="md" />
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('admin.codexTurnState.overview.totalModels') }}
              </p>
              <p class="text-xl font-semibold text-gray-900 dark:text-white">
                {{ overview?.total_models ?? 0 }}
              </p>
            </div>
          </div>
        </div>

        <!-- Healthy Models -->
        <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="flex items-center gap-3">
            <div class="flex h-10 w-10 items-center justify-center rounded-lg bg-emerald-50 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400">
              <Icon name="checkCircle" size="md" />
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('admin.codexTurnState.overview.healthyModels') }}
              </p>
              <p class="text-xl font-semibold text-emerald-600 dark:text-emerald-400">
                {{ overview?.healthy_models ?? 0 }}
              </p>
            </div>
          </div>
        </div>

        <!-- Expiring Soon -->
        <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="flex items-center gap-3">
            <div class="flex h-10 w-10 items-center justify-center rounded-lg bg-amber-50 text-amber-600 dark:bg-amber-900/30 dark:text-amber-400">
              <Icon name="clock" size="md" />
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('admin.codexTurnState.overview.expiringModels') }}
              </p>
              <p class="text-xl font-semibold text-amber-600 dark:text-amber-400">
                {{ overview?.expiring_models ?? 0 }}
              </p>
            </div>
          </div>
        </div>

        <!-- Degraded Models -->
        <div class="rounded-xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="flex items-center gap-3">
            <div class="flex h-10 w-10 items-center justify-center rounded-lg bg-rose-50 text-rose-600 dark:bg-rose-900/30 dark:text-rose-400">
              <Icon name="exclamationTriangle" size="md" />
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('admin.codexTurnState.overview.degradedModels') }}
              </p>
              <p class="text-xl font-semibold text-rose-600 dark:text-rose-400">
                {{ overview?.degraded_models ?? 0 }}
              </p>
            </div>
          </div>
        </div>
      </div>

      <!-- Filters & Search Bar -->
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="flex flex-wrap items-center gap-3">
          <!-- Search Input -->
          <div class="relative w-full sm:w-64">
            <Icon
              name="search"
              size="sm"
              class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
            />
            <input
              v-model="searchQuery"
              type="text"
              class="input pl-9 text-xs"
              :placeholder="t('admin.codexTurnState.filters.searchPlaceholder')"
            />
          </div>

          <!-- Status Filter -->
          <div class="w-full sm:w-40">
            <select v-model="statusFilter" class="input text-xs">
              <option value="all">{{ t('admin.codexTurnState.filters.allStatus') }}</option>
              <option value="healthy">{{ t('admin.codexTurnState.filters.healthy') }}</option>
              <option value="expiring">{{ t('admin.codexTurnState.filters.expiring') }}</option>
              <option value="degraded">{{ t('admin.codexTurnState.filters.degraded') }}</option>
              <option value="expired">{{ t('admin.codexTurnState.filters.expired') }}</option>
              <option value="paused">{{ t('admin.codexTurnState.filters.paused') }}</option>
            </select>
          </div>
        </div>

        <div class="text-xs text-gray-400">
          Updated: {{ overview?.updated_at ? formatDateTime(overview.updated_at) : '-' }}
        </div>
      </div>

      <!-- Loading State -->
      <div v-if="loading && !overview" class="flex h-64 items-center justify-center">
        <Icon name="refresh" size="lg" class="animate-spin text-primary-600" />
      </div>

      <!-- Empty State -->
      <div
        v-else-if="filteredAccounts.length === 0"
        class="rounded-xl border border-gray-200 bg-white p-12 text-center shadow-sm dark:border-dark-700 dark:bg-dark-800"
      >
        <EmptyState
          :title="t('admin.codexTurnState.table.empty')"
          :action-text="t('admin.codexTurnState.actions.addModel')"
          @action="openAddModal"
        />
      </div>

      <!-- Accounts & Model Slots List -->
      <div v-else class="space-y-6">
        <div
          v-for="account in filteredAccounts"
          :key="account.account_id"
          class="overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800"
        >
          <!-- Account Header -->
          <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 bg-gray-50/50 px-5 py-4 dark:border-dark-700 dark:bg-dark-800/80">
            <div class="flex items-center gap-3">
              <div class="flex h-9 w-9 items-center justify-center rounded-lg bg-primary-50 text-primary-600 dark:bg-primary-900/30 dark:text-primary-400">
                <Icon name="globe" size="md" />
              </div>
              <div>
                <div class="flex items-center gap-2">
                  <h3 class="text-base font-semibold text-gray-900 dark:text-white">
                    {{ account.account_name }}
                  </h3>
                  <span class="text-xs text-gray-400">#{{ account.account_id }}</span>
                  <PlatformTypeBadge :platform="account.platform" :type="account.type" />
                </div>
              </div>
            </div>
            <div class="flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
              <span class="badge" :class="account.status === 'active' ? 'badge-success' : 'badge-danger'">
                {{ account.status }}
              </span>
              <span>{{ account.models.length }} slots</span>
            </div>
          </div>

          <!-- Slots Table -->
          <div class="overflow-x-auto">
            <table class="w-full text-left text-sm">
              <thead class="bg-gray-50 text-xs uppercase text-gray-500 dark:bg-dark-900/40 dark:text-gray-400">
                <tr>
                  <th class="px-5 py-3.5">{{ t('admin.codexTurnState.table.model') }}</th>
                  <th class="px-5 py-3.5">{{ t('admin.codexTurnState.table.stateStatus') }}</th>
                  <th class="px-5 py-3.5">{{ t('admin.codexTurnState.table.ttl') }}</th>
                  <th class="px-5 py-3.5">{{ t('admin.codexTurnState.table.degradation') }}</th>
                  <th class="px-5 py-3.5">{{ t('admin.codexTurnState.table.lastProbe') }}</th>
                  <th class="px-5 py-3.5 text-right">{{ t('admin.codexTurnState.table.operations') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr
                  v-for="slot in account.models"
                  :key="slot.model"
                  class="transition-colors hover:bg-gray-50/50 dark:hover:bg-dark-700/50"
                  :class="{ 'opacity-60': !slot.enabled }"
                >
                  <!-- Model Info -->
                  <td class="px-5 py-4">
                    <div class="flex items-center gap-2">
                      <div class="font-medium text-gray-900 dark:text-white">
                        {{ slot.model }}
                      </div>
                      <span v-if="!slot.enabled" class="badge badge-secondary text-[10px]">
                        {{ t('admin.codexTurnState.status.paused') }}
                      </span>
                    </div>
                    <div class="text-xs text-gray-400">
                      Target: {{ slot.target_state_len }}B &middot; Advance: {{ slot.refresh_advance_minutes }}m &middot; Renewal: {{ slot.cookie_renewal_seconds || 150 }}s
                    </div>
                  </td>

                  <!-- Turn-State Status -->
                  <td class="px-5 py-4">
                    <div class="flex flex-col gap-1.5">
                      <div class="flex items-center gap-2">
                        <span :class="['badge', getStateBadgeClass(slot)]">
                          {{ getStateStatusLabel(slot) }}
                        </span>
                        <span v-if="slot.inspection?.length" class="text-xs font-mono text-gray-500">
                          {{ slot.inspection.length }}B
                        </span>
                      </div>
                      <div v-if="slot.inspection?.valid" class="text-xs text-gray-400">
                        Expires: {{ formatDateTime(slot.inspection.expires_at) }}
                      </div>
                    </div>
                  </td>

                  <!-- Remaining TTL & Progress Bar -->
                  <td class="px-5 py-4 min-w-[160px]">
                    <div v-if="slot.inspection?.valid" class="space-y-1.5">
                      <div class="space-y-0.5">
                        <div class="flex justify-between text-xs font-medium">
                          <span :class="getTtlTextClass(slot.inspection.remaining_seconds)">
                            State: {{ formatTtl(slot.inspection.remaining_seconds, slot.inspection.is_expired) }}
                          </span>
                          <span class="text-gray-400">
                            {{ getTtlPercent(slot.inspection.remaining_seconds) }}%
                          </span>
                        </div>
                        <div class="h-1.5 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-dark-600">
                          <div
                            class="h-full transition-all duration-500"
                            :class="getTtlBarClass(slot.inspection.remaining_seconds)"
                            :style="{ width: `${getTtlPercent(slot.inspection.remaining_seconds)}%` }"
                          ></div>
                        </div>
                      </div>

                      <!-- Cookie 240s / 150s Status -->
                      <div class="flex items-center justify-between text-[11px] pt-0.5 border-t border-gray-100 dark:border-dark-700">
                        <span class="text-gray-400">LB Cookie:</span>
                        <span v-if="slot.cflb && slot.oailb" :class="getCookieStatusClass(slot)">
                          {{ getCookieStatusText(slot) }}
                        </span>
                        <span v-else class="text-amber-500 font-medium">
                          {{ t('admin.codexTurnState.status.cookieMissing') }}
                        </span>
                      </div>
                    </div>
                    <span v-else class="text-xs text-gray-400">-</span>
                  </td>

                  <!-- Degradation Status -->
                  <td class="px-5 py-4">
                    <div class="flex flex-col gap-1">
                      <span :class="['badge', getDegradationBadgeClass(slot.degradation_status)]">
                        {{ getDegradationStatusLabel(slot.degradation_status) }}
                      </span>
                      <div
                        v-if="slot.degradation_status === 'degraded' && slot.recent_degradation"
                        class="text-xs text-rose-500"
                      >
                        <div>{{ slot.recent_degradation.requested_model }} &rarr; {{ slot.recent_degradation.response_model }}</div>
                        <div class="text-[11px] text-gray-400">
                          Count: {{ slot.recent_degradation.count }} &middot; {{ formatRelativeTime(slot.recent_degradation.last_seen) }}
                        </div>
                      </div>
                    </div>
                  </td>

                  <!-- Last Probe Details -->
                  <td class="px-5 py-4">
                    <div v-if="slot.last_probe" class="flex flex-col gap-0.5 text-xs">
                      <div class="flex items-center gap-1.5">
                        <span
                          class="inline-block h-2 w-2 rounded-full"
                          :class="slot.last_probe.success ? 'bg-emerald-500' : 'bg-rose-500'"
                        ></span>
                        <span class="font-medium text-gray-700 dark:text-gray-200">
                          {{ slot.last_probe.status_code }}
                        </span>
                        <span class="text-gray-400">
                          ({{ slot.last_probe.latency_ms }}ms)
                        </span>
                        <span v-if="slot.in_cooldown" class="badge badge-warning text-[10px]">
                          {{ t('admin.codexTurnState.status.inCooldown') }}
                        </span>
                      </div>
                      <div class="text-[11px] text-gray-400">
                        {{ formatRelativeTime(slot.last_probe.at) }}
                        <span v-if="slot.last_probe.proxy_used" class="truncate max-w-[120px] inline-block align-bottom ml-1">
                          via {{ slot.last_probe.proxy_used }}
                        </span>
                      </div>
                      <div v-if="slot.last_probe.error_message" class="text-[11px] text-rose-500 truncate max-w-[180px]">
                        {{ slot.last_probe.error_message }}
                      </div>
                    </div>
                    <span v-else class="text-xs text-gray-400">-</span>
                  </td>

                  <!-- Action Buttons -->
                  <td class="px-5 py-4 text-right">
                    <div class="flex items-center justify-end gap-1.5">
                      <!-- Pause / Resume Toggle -->
                      <button
                        type="button"
                        class="btn btn-secondary btn-sm"
                        @click="handleToggleSlot(account.account_id, slot.model, !slot.enabled)"
                        :title="slot.enabled ? t('admin.codexTurnState.actions.pause') : t('admin.codexTurnState.actions.resume')"
                      >
                        <Icon :name="slot.enabled ? 'ban' : 'play'" size="xs" />
                      </button>

                      <!-- Probe Now -->
                      <button
                        type="button"
                        class="btn btn-secondary btn-sm"
                        :disabled="isSlotProbing(account.account_id, slot.model)"
                        @click="handleProbeSlot(account.account_id, slot.model)"
                        :title="t('admin.codexTurnState.actions.probeNow')"
                      >
                        <Icon
                          name="play"
                          size="xs"
                          :class="{ 'animate-spin': isSlotProbing(account.account_id, slot.model) }"
                        />
                        <span>{{ t('admin.codexTurnState.actions.probeNow') }}</span>
                      </button>

                      <!-- History -->
                      <button
                        type="button"
                        class="btn btn-secondary btn-sm"
                        @click="openHistoryModal(account.account_id, slot.model)"
                        :title="t('admin.codexTurnState.historyDrawer.title')"
                      >
                        <Icon name="clock" size="xs" />
                      </button>

                      <!-- Remove -->
                      <button
                        type="button"
                        class="btn btn-danger btn-sm"
                        @click="confirmRemoveSlot(account, slot.model)"
                        :title="t('admin.codexTurnState.actions.remove')"
                      >
                        <Icon name="trash" size="xs" />
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

    <!-- Add Monitored Model Modal -->
    <BaseDialog
      :show="showAddModal"
      :title="t('admin.codexTurnState.modal.addTitle')"
      width="normal"
      @close="closeAddModal"
    >
      <form id="add-model-form" @submit.prevent="handleAddModel" class="space-y-4">
        <!-- Account Selection -->
        <div>
          <label class="input-label">{{ t('admin.codexTurnState.modal.accountLabel') }}</label>
          <select
            v-model="addForm.account_id"
            required
            class="input"
          >
            <option :value="0" disabled>{{ t('admin.codexTurnState.modal.accountPlaceholder') }}</option>
            <option
              v-for="acc in openAIAccounts"
              :key="acc.id"
              :value="acc.id"
            >
              {{ acc.name }} (#{{ acc.id }}, {{ acc.type }})
            </option>
          </select>
        </div>

        <!-- Model Input + Quick Picks -->
        <div>
          <label class="input-label">{{ t('admin.codexTurnState.modal.modelLabel') }}</label>
          <input
            v-model="addForm.model"
            type="text"
            required
            class="input font-mono"
            :placeholder="t('admin.codexTurnState.modal.modelPlaceholder')"
          />
          <div class="mt-2 flex flex-wrap items-center gap-1.5">
            <span class="text-xs text-gray-500">{{ t('admin.codexTurnState.modal.quickModels') }}:</span>
            <button
              v-for="m in quickModelSuggestions"
              :key="m"
              type="button"
              class="rounded bg-gray-100 px-2 py-0.5 text-xs font-mono text-gray-700 hover:bg-primary-50 hover:text-primary-600 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-primary-900/30 dark:hover:text-primary-400"
              @click="addForm.model = m"
            >
              {{ m }}
            </button>
          </div>
        </div>

        <!-- Target State Length -->
        <div>
          <label class="input-label">{{ t('admin.codexTurnState.modal.targetLenLabel') }}</label>
          <input
            v-model.number="addForm.target_state_len"
            type="number"
            class="input"
            :placeholder="t('admin.codexTurnState.modal.targetLenPlaceholder')"
          />
        </div>

        <!-- Advance Minutes -->
        <div>
          <label class="input-label">{{ t('admin.codexTurnState.modal.advanceMinutesLabel') }}</label>
          <input
            v-model.number="addForm.refresh_advance_minutes"
            type="number"
            class="input"
            :placeholder="t('admin.codexTurnState.modal.advanceMinutesPlaceholder')"
          />
        </div>

        <!-- Cookie Advance Renewal Seconds -->
        <div>
          <label class="input-label">{{ t('admin.codexTurnState.modal.cookieRenewalLabel') }}</label>
          <input
            v-model.number="addForm.cookie_renewal_seconds"
            type="number"
            class="input"
            :placeholder="t('admin.codexTurnState.modal.cookieRenewalPlaceholder')"
          />
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeAddModal">
            {{ t('admin.codexTurnState.modal.cancel') }}
          </button>
          <button
            type="submit"
            form="add-model-form"
            class="btn btn-primary"
            :disabled="addingModel || !addForm.account_id || !addForm.model"
          >
            <Icon v-if="addingModel" name="refresh" size="sm" class="animate-spin mr-1.5" />
            <span>{{ t('admin.codexTurnState.modal.submit') }}</span>
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Probe Proxies Pool Modal -->
    <BaseDialog
      :show="showProxiesModal"
      :title="t('admin.codexTurnState.proxiesModal.title')"
      width="normal"
      @close="showProxiesModal = false"
    >
      <div class="space-y-4">
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.codexTurnState.proxiesModal.hint') }}
        </p>

        <!-- Advance Renewal Seconds -->
        <div>
          <label class="input-label font-medium text-gray-700 dark:text-gray-200">
            {{ t('admin.codexTurnState.proxiesModal.renewalLabel') }}
          </label>
          <p class="text-[11px] text-gray-400 mb-1.5">
            {{ t('admin.codexTurnState.proxiesModal.renewalHint') }}
          </p>
          <input
            v-model.number="cookieRenewalSeconds"
            type="number"
            min="10"
            class="input font-mono text-xs"
            :placeholder="t('admin.codexTurnState.proxiesModal.renewalPlaceholder')"
          />
        </div>

        <!-- Static Proxies Section -->
        <div>
          <label class="input-label font-medium text-gray-700 dark:text-gray-200">
            {{ t('admin.codexTurnState.proxiesModal.staticLabel') }}
          </label>
          <p class="text-[11px] text-gray-400 mb-1.5">
            {{ t('admin.codexTurnState.proxiesModal.staticHint') }}
          </p>
          <textarea
            v-model="staticProxiesText"
            rows="4"
            class="input font-mono text-xs leading-relaxed"
            :placeholder="t('admin.codexTurnState.proxiesModal.staticPlaceholder')"
          ></textarea>
        </div>

        <!-- Dynamic Proxies Section -->
        <div>
          <label class="input-label font-medium text-gray-700 dark:text-gray-200">
            {{ t('admin.codexTurnState.proxiesModal.dynamicLabel') }}
          </label>
          <p class="text-[11px] text-gray-400 mb-1.5">
            {{ t('admin.codexTurnState.proxiesModal.dynamicHint') }}
          </p>
          <textarea
            v-model="dynamicProxiesText"
            rows="3"
            class="input font-mono text-xs leading-relaxed"
            :placeholder="t('admin.codexTurnState.proxiesModal.dynamicPlaceholder')"
          ></textarea>
        </div>
      </div>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="showProxiesModal = false">
            {{ t('admin.codexTurnState.modal.cancel') }}
          </button>
          <button
            type="button"
            class="btn btn-primary"
            :disabled="savingProxies"
            @click="handleSaveProxies"
          >
            <Icon v-if="savingProxies" name="refresh" size="sm" class="animate-spin mr-1.5" />
            <span>{{ t('admin.codexTurnState.proxiesModal.save') }}</span>
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Slot History Modal -->
    <BaseDialog
      :show="showHistoryModal"
      :title="`${t('admin.codexTurnState.historyDrawer.title')} (${currentHistoryKey})`"
      width="wide"
      @close="showHistoryModal = false"
    >
      <div class="flex items-center justify-between pb-3 border-b border-gray-100 dark:border-dark-700">
        <span class="text-xs text-gray-500">
          {{ currentSlotHistory.length }} entries recorded
        </span>
        <button
          v-if="currentSlotHistory.length > 0"
          type="button"
          class="btn btn-danger btn-sm text-xs"
          @click="showClearHistoryConfirm = true"
        >
          <Icon name="trash" size="xs" />
          <span>{{ t('admin.codexTurnState.historyDrawer.clearAction') }}</span>
        </button>
      </div>

      <div v-if="currentSlotHistory.length === 0" class="py-8 text-center text-sm text-gray-400">
        {{ t('admin.codexTurnState.historyDrawer.empty') }}
      </div>
      <div v-else class="max-h-96 overflow-y-auto pt-3">
        <table class="w-full text-left text-xs">
          <thead class="sticky top-0 bg-gray-50 uppercase text-gray-500 dark:bg-dark-900">
            <tr>
              <th class="px-4 py-2.5">Time</th>
              <th class="px-4 py-2.5">Status</th>
              <th class="px-4 py-2.5">Latency</th>
              <th class="px-4 py-2.5">State Len</th>
              <th class="px-4 py-2.5">Proxy</th>
              <th class="px-4 py-2.5">Detail</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr
              v-for="(item, idx) in currentSlotHistory"
              :key="idx"
              class="hover:bg-gray-50/50 dark:hover:bg-dark-700/50"
            >
              <td class="px-4 py-2 whitespace-nowrap text-gray-500">
                {{ formatDateTime(item.at) }}
              </td>
              <td class="px-4 py-2 whitespace-nowrap">
                <span
                  class="inline-flex items-center gap-1 font-medium"
                  :class="item.success ? 'text-emerald-600' : 'text-rose-600'"
                >
                  <span class="h-1.5 w-1.5 rounded-full" :class="item.success ? 'bg-emerald-500' : 'bg-rose-500'"></span>
                  {{ item.status_code }}
                </span>
              </td>
              <td class="px-4 py-2 whitespace-nowrap text-gray-500 font-mono">
                {{ item.latency_ms }}ms
              </td>
              <td class="px-4 py-2 whitespace-nowrap font-mono text-gray-600 dark:text-gray-300">
                {{ item.state_len }}B
              </td>
              <td class="px-4 py-2 text-gray-400 truncate max-w-[140px]">
                {{ item.proxy_used || '-' }}
              </td>
              <td class="px-4 py-2 text-rose-500 truncate max-w-[200px]">
                {{ item.error_message || '-' }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <template #footer>
        <div class="flex justify-end">
          <button type="button" class="btn btn-secondary" @click="showHistoryModal = false">
            {{ t('common.close') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Clear History Confirm Dialog -->
    <ConfirmDialog
      :show="showClearHistoryConfirm"
      :title="t('admin.codexTurnState.historyDrawer.clearAction')"
      :message="t('admin.codexTurnState.historyDrawer.clearConfirm')"
      :danger="true"
      @confirm="handleClearHistory"
      @cancel="showClearHistoryConfirm = false"
    />

    <!-- Remove Confirm Dialog -->
    <ConfirmDialog
      :show="showRemoveConfirm"
      :title="t('admin.codexTurnState.actions.remove')"
      :message="removeConfirmMessage"
      :danger="true"
      @confirm="handleConfirmRemove"
      @cancel="showRemoveConfirm = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import {
  codexTurnStateApi,
  type CodexTurnStateOverview,
  type CodexAccountState,
  type ModelSlotStatus,
  type ProbeHistoryItem,
} from '@/api/admin/codexTurnState'
import { adminAPI } from '@/api/admin'
import type { AccountListItem } from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import PlatformTypeBadge from '@/components/common/PlatformTypeBadge.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime, formatRelativeTime } from '@/utils/format'

const { t } = useI18n()
const appStore = useAppStore()

// State
const loading = ref(false)
const autoRefresh = ref(false)
let autoRefreshTimer: ReturnType<typeof setInterval> | null = null

const searchQuery = ref('')
const statusFilter = ref('all')

const overview = ref<CodexTurnStateOverview | null>(null)
const probingSlots = reactive(new Set<string>())
const openAIAccounts = ref<AccountListItem[]>([])

// Quick model recommendations
const quickModelSuggestions = ['gpt-6-astra', 'gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.4-orion']

// Add Model Modal State
const showAddModal = ref(false)
const addingModel = ref(false)
const addForm = reactive({
  account_id: 0,
  model: '',
  target_state_len: 292,
  refresh_advance_minutes: 15,
  cookie_renewal_seconds: undefined as number | undefined,
})

// Proxies Modal State
const showProxiesModal = ref(false)
const savingProxies = ref(false)
const staticProxiesText = ref('')
const dynamicProxiesText = ref('')
const cookieRenewalSeconds = ref<number | undefined>(150)

// History Modal State
const showHistoryModal = ref(false)
const showClearHistoryConfirm = ref(false)
const currentHistoryAccountID = ref<number>(0)
const currentHistoryModel = ref<string>('')
const currentHistoryKey = ref('')
const currentSlotHistory = ref<ProbeHistoryItem[]>([])
const allHistory = ref<Record<string, ProbeHistoryItem[]>>({})

// Remove Confirmation State
const showRemoveConfirm = ref(false)
const slotToRemove = ref<{ accountId: number; accountName: string; model: string } | null>(null)

const removeConfirmMessage = computed(() => {
  if (!slotToRemove.value) return ''
  return t('admin.codexTurnState.messages.confirmRemove', {
    account: slotToRemove.value.accountName,
    model: slotToRemove.value.model,
  })
})

function slotKey(accountId: number, model: string): string {
  return `${accountId}:${model}`
}

function isSlotProbing(accountId: number, model: string): boolean {
  return probingSlots.has(slotKey(accountId, model))
}

const hasNoProxies = computed(() => {
  if (!overview.value) return false
  const staticCount = overview.value.static_proxy_count ?? 0
  const dynamicCount = overview.value.dynamic_proxy_count ?? 0
  return staticCount + dynamicCount === 0
})

// Filtered Accounts
const filteredAccounts = computed(() => {
  if (!overview.value?.accounts) return []
  const query = searchQuery.value.trim().toLowerCase()
  const filter = statusFilter.value

  return overview.value.accounts
    .map((acc) => {
      const matchAccount = !query || acc.account_name.toLowerCase().includes(query) || String(acc.account_id).includes(query)
      const filteredSlots = acc.models.filter((slot) => {
        const matchModel = matchAccount || slot.model.toLowerCase().includes(query)
        if (!matchModel) return false

        if (filter === 'all') return true
        if (filter === 'paused') return !slot.enabled
        if (filter === 'degraded') return slot.degradation_status === 'degraded'
        if (filter === 'expired') return slot.inspection?.is_expired
        if (filter === 'expiring') return slot.inspection?.valid && !slot.inspection.is_expired && slot.inspection.remaining_seconds < 900
        if (filter === 'healthy') return slot.inspection?.valid && !slot.inspection.is_expired && slot.inspection.remaining_seconds >= 900
        return true
      })
      return {
        ...acc,
        models: filteredSlots,
      }
    })
    .filter((acc) => acc.models.length > 0)
})

// Auto-Refresh
function toggleAutoRefresh() {
  autoRefresh.value = !autoRefresh.value
  if (autoRefresh.value) {
    autoRefreshTimer = setInterval(() => {
      loadData()
    }, 30000)
  } else if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
}

// Data loaders
async function loadData() {
  loading.value = true
  try {
    const { data } = await codexTurnStateApi.getState()
    overview.value = data
  } catch (error: any) {
    appStore.showError(error?.message || 'Failed to load Codex Turn-State overview')
  } finally {
    loading.value = false
  }
}

async function loadOpenAIAccounts() {
  try {
    const res = await adminAPI.accounts.list(1, 100, { platform: 'openai' })
    openAIAccounts.value = res.items || []
  } catch (err) {
    console.error('Failed to load OpenAI accounts', err)
  }
}

// Status helpers
function getStateBadgeClass(slot: ModelSlotStatus): string {
  if (!slot.inspection || !slot.inspection.valid) {
    return 'badge-secondary'
  }
  if (slot.inspection.is_expired) {
    return 'badge-danger'
  }
  const renewalSec = slot.cookie_renewal_seconds || 150
  if (slot.inspection.remaining_seconds < 900 || (slot.cookie_age_seconds !== undefined && slot.cookie_age_seconds >= renewalSec)) {
    return 'badge-warning'
  }
  return 'badge-success'
}

function getStateStatusLabel(slot: ModelSlotStatus): string {
  if (!slot.inspection || !slot.inspection.valid) {
    return t('admin.codexTurnState.status.missing')
  }
  if (slot.inspection.is_expired) {
    return t('admin.codexTurnState.status.expired')
  }
  const renewalSec = slot.cookie_renewal_seconds || 150
  if (slot.inspection.remaining_seconds < 900 || (slot.cookie_age_seconds !== undefined && slot.cookie_age_seconds >= renewalSec)) {
    return t('admin.codexTurnState.status.expiring')
  }
  return t('admin.codexTurnState.status.healthy')
}

function getCookieStatusClass(slot: ModelSlotStatus): string {
  const renewalSec = slot.cookie_renewal_seconds || 150
  const age = slot.cookie_age_seconds ?? 0
  if (age >= renewalSec) {
    return 'text-amber-500 font-medium'
  }
  return 'text-emerald-600 dark:text-emerald-400 font-mono'
}

function getCookieStatusText(slot: ModelSlotStatus): string {
  const renewalSec = slot.cookie_renewal_seconds || 150
  const rem = slot.cookie_remaining_seconds ?? 0
  const age = slot.cookie_age_seconds ?? 0
  if (age >= renewalSec) {
    return `${t('admin.codexTurnState.status.cookieExpiring')} (${age}s / ${renewalSec}s)`
  }
  return `${t('admin.codexTurnState.status.cookieActive')} (${rem}s)`
}

function getDegradationBadgeClass(status: string): string {
  switch (status) {
    case 'no_record':
      return 'badge-success'
    case 'handled':
      return 'badge-info'
    case 'degraded':
      return 'badge-danger'
    default:
      return 'badge-secondary'
  }
}

function getDegradationStatusLabel(status: string): string {
  switch (status) {
    case 'no_record':
      return t('admin.codexTurnState.status.noRecord')
    case 'handled':
      return t('admin.codexTurnState.status.handled')
    case 'degraded':
      return t('admin.codexTurnState.status.degraded')
    default:
      return status
  }
}

function formatTtl(remainingSeconds?: number, isExpired?: boolean): string {
  if (remainingSeconds === undefined || remainingSeconds === null) return '-'
  if (isExpired || remainingSeconds <= 0) return t('admin.codexTurnState.status.expired')
  const mins = Math.floor(remainingSeconds / 60)
  const secs = remainingSeconds % 60
  if (mins >= 60) {
    const hours = Math.floor(mins / 60)
    const remMins = mins % 60
    return `${hours}h ${remMins}m`
  }
  return `${mins}m ${secs}s`
}

function getTtlPercent(remainingSeconds?: number): number {
  if (!remainingSeconds || remainingSeconds <= 0) return 0
  return Math.min(100, Math.max(0, Math.round((remainingSeconds / 3600) * 100)))
}

function getTtlBarClass(remainingSeconds?: number): string {
  if (!remainingSeconds || remainingSeconds <= 0) return 'bg-rose-500'
  if (remainingSeconds < 900) return 'bg-amber-500'
  return 'bg-emerald-500'
}

function getTtlTextClass(remainingSeconds?: number): string {
  if (!remainingSeconds || remainingSeconds <= 0) return 'text-rose-600 dark:text-rose-400'
  if (remainingSeconds < 900) return 'text-amber-600 dark:text-amber-400'
  return 'text-emerald-600 dark:text-emerald-400'
}

// Probing action
async function handleProbeSlot(accountId: number, model: string) {
  if (hasNoProxies.value) {
    appStore.showWarning(t('admin.codexTurnState.noProxyWarning.description'))
    openProxiesModal()
    return
  }
  const key = slotKey(accountId, model)
  probingSlots.add(key)
  try {
    const res = await codexTurnStateApi.probeModel({ account_id: accountId, model, force: true })
    if (res.data?.success) {
      appStore.showSuccess(t('admin.codexTurnState.messages.probeSuccess'))
    } else {
      appStore.showError(res.data?.error_message || t('admin.codexTurnState.messages.probeFailed'))
    }
    await loadData()
  } catch (error: any) {
    appStore.showError(error?.response?.data?.message || error?.message || t('admin.codexTurnState.messages.probeFailed'))
  } finally {
    probingSlots.delete(key)
  }
}

// Toggle slot pause/resume
async function handleToggleSlot(accountId: number, model: string, enabled: boolean) {
  try {
    await codexTurnStateApi.toggleMonitoredModel(accountId, model, enabled)
    appStore.showSuccess(t('admin.codexTurnState.messages.statusUpdated'))
    await loadData()
  } catch (error: any) {
    appStore.showError(error?.message || 'Failed to update slot status')
  }
}

// Add Model modal
function openAddModal() {
  addForm.account_id = openAIAccounts.value.length > 0 ? openAIAccounts.value[0].id : 0
  addForm.model = quickModelSuggestions[0]
  addForm.target_state_len = 292
  addForm.refresh_advance_minutes = 15
  addForm.cookie_renewal_seconds = undefined
  showAddModal.value = true
  if (openAIAccounts.value.length === 0) {
    loadOpenAIAccounts()
  }
}

function closeAddModal() {
  showAddModal.value = false
}

async function handleAddModel() {
  if (!addForm.account_id || !addForm.model) return
  addingModel.value = true
  try {
    await codexTurnStateApi.addMonitoredModel({
      account_id: addForm.account_id,
      model: addForm.model.trim(),
      target_state_len: addForm.target_state_len || 292,
      refresh_advance_minutes: addForm.refresh_advance_minutes || 15,
      cookie_renewal_seconds: addForm.cookie_renewal_seconds ? Number(addForm.cookie_renewal_seconds) : undefined,
    })
    appStore.showSuccess(t('admin.codexTurnState.messages.modelAdded'))
    closeAddModal()
    await loadData()
  } catch (error: any) {
    appStore.showError(error?.response?.data?.message || error?.message || 'Failed to add monitored model')
  } finally {
    addingModel.value = false
  }
}

// Remove slot
function confirmRemoveSlot(account: CodexAccountState, model: string) {
  slotToRemove.value = {
    accountId: account.account_id,
    accountName: account.account_name,
    model,
  }
  showRemoveConfirm.value = true
}

async function handleConfirmRemove() {
  if (!slotToRemove.value) return
  const { accountId, model } = slotToRemove.value
  showRemoveConfirm.value = false
  try {
    await codexTurnStateApi.removeMonitoredModel(accountId, model)
    appStore.showSuccess(t('admin.codexTurnState.messages.modelRemoved'))
    await loadData()
  } catch (error: any) {
    appStore.showError(error?.response?.data?.message || error?.message || 'Failed to remove model')
  } finally {
    slotToRemove.value = null
  }
}

// Proxies Modal
async function openProxiesModal() {
  try {
    const res = await codexTurnStateApi.getProxies()
    staticProxiesText.value = (res.data?.static_proxies || []).join('\n')
    dynamicProxiesText.value = (res.data?.dynamic_proxies || []).join('\n')
    cookieRenewalSeconds.value = res.data?.cookie_renewal_seconds || 150
  } catch (err) {
    console.error('Failed to load proxies', err)
  }
  showProxiesModal.value = true
}

async function handleSaveProxies() {
  savingProxies.value = true
  try {
    const staticList = staticProxiesText.value
      .split('\n')
      .map(s => s.trim())
      .filter(Boolean)
    const dynamicList = dynamicProxiesText.value
      .split('\n')
      .map(s => s.trim())
      .filter(Boolean)
    await codexTurnStateApi.saveProxies({
      static_proxies: staticList,
      dynamic_proxies: dynamicList,
      cookie_renewal_seconds: cookieRenewalSeconds.value ? Number(cookieRenewalSeconds.value) : 150,
    })
    appStore.showSuccess(t('admin.codexTurnState.proxiesModal.success'))
    showProxiesModal.value = false
    await loadData()
  } catch (error: any) {
    appStore.showError(error?.response?.data?.message || error?.message || 'Failed to save proxies')
  } finally {
    savingProxies.value = false
  }
}

// History Modal
async function openHistoryModal(accountId: number, model: string) {
  currentHistoryAccountID.value = accountId
  currentHistoryModel.value = model
  currentHistoryKey.value = `${accountId}:${model}`
  try {
    const res = await codexTurnStateApi.getHistory()
    allHistory.value = res.data || {}
    currentSlotHistory.value = allHistory.value[currentHistoryKey.value] || []
  } catch (err) {
    console.error('Failed to load probe history', err)
    currentSlotHistory.value = []
  }
  showHistoryModal.value = true
}

async function handleClearHistory() {
  showClearHistoryConfirm.value = false
  try {
    await codexTurnStateApi.clearHistory(currentHistoryAccountID.value, currentHistoryModel.value)
    currentSlotHistory.value = []
    appStore.showSuccess(t('admin.codexTurnState.historyDrawer.clearSuccess'))
  } catch (error: any) {
    appStore.showError(error?.message || 'Failed to clear probe history')
  }
}

onMounted(() => {
  loadData()
  loadOpenAIAccounts()
})

onUnmounted(() => {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
})
</script>
