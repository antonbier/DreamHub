<!--
  AdminHub's terminal (docs/ADMINHUB_PLAN.md, A4, #36): not built by
  DreamHub, a proxy in front of the box's existing shellinaboxd. Off by
  default; enabling it needs DreamHub's own password protection turned on
  (Settings) and the password typed again right now — the one explicit
  exception to preview-and-confirm (PRINCIPLES.md), since there's nothing
  to preview about opening a shell. Once enabled, it's just an iframe onto
  the proxy path; ShellInABox still asks for its own OS login inside.
-->
<template>
  <section class="mt-5 rounded border border-line bg-tile p-4">
    <h2 class="text-[14px] font-semibold text-text">{{ t('admin.terminal.title') }}</h2>

    <p v-if="loading" class="mt-2 text-[13px] text-faint">{{ t('common.loading') }}</p>

    <p v-else-if="!status?.available" class="mt-2 text-[13px] text-faint">{{ t('admin.terminal.unavailable') }}</p>

    <template v-else-if="!status.enabled">
      <p class="mt-2 text-[13px] text-warn">{{ t('admin.terminal.warning') }}</p>
      <p v-if="!status.authOn" class="mt-2 text-[13px] text-muted">
        {{ t('admin.terminal.needsAuth') }}
        <RouterLink to="/settings/access" class="text-accent hover:underline">{{ t('admin.terminal.openSettings') }}</RouterLink>
      </p>
      <form v-else class="mt-3 flex flex-wrap items-end gap-2" @submit.prevent="enable">
        <label class="flex flex-col gap-1">
          <span class="text-[12.5px] text-muted">{{ t('admin.terminal.passwordLabel') }}</span>
          <input
            v-model="password"
            type="password"
            autocomplete="current-password"
            class="w-56 rounded border border-line bg-field px-2.5 py-1.5 text-[13px] text-text"
          />
        </label>
        <button type="submit" class="rounded bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg disabled:opacity-60" :disabled="enabling || !password">
          {{ t('admin.terminal.enable') }}
        </button>
      </form>
      <p v-if="enableError" class="mt-2 text-[12.5px] text-warn" role="alert">{{ enableError }}</p>
    </template>

    <template v-else>
      <div class="mt-2 flex items-center justify-between gap-3">
        <span class="flex items-center gap-1.5 text-[13px] text-warn">
          <span class="h-1.5 w-1.5 rounded-full bg-warn" />
          {{ t('admin.terminal.enabledWarning') }}
        </span>
        <button type="button" class="rounded border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field" @click="disable">
          {{ t('admin.terminal.disable') }}
        </button>
      </div>
      <iframe :src="proxyPath" class="mt-3 h-96 w-full rounded border border-line bg-black" :title="t('admin.terminal.title')" />
    </template>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { api, ApiError } from '@/api/client'
import type { TerminalStatus } from '@/api/types'

const { t } = useI18n()

const status = ref<TerminalStatus | null>(null)
const loading = ref(true)
const proxyPath = api.admin.terminalProxyPath

async function load() {
  try {
    status.value = await api.admin.terminalStatus()
  } finally {
    loading.value = false
  }
}

const password = ref('')
const enabling = ref(false)
const enableError = ref('')

async function enable() {
  enabling.value = true
  enableError.value = ''
  try {
    await api.admin.enableTerminal(password.value)
    password.value = ''
    await load()
  } catch (err) {
    enableError.value = err instanceof ApiError && err.code === 'wrong_password' ? t('admin.terminal.wrongPassword') : t('admin.terminal.enableFailed')
  } finally {
    enabling.value = false
  }
}

async function disable() {
  await api.admin.disableTerminal()
  await load()
}

onMounted(load)
</script>
