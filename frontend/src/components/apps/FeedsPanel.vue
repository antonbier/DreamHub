<!--
  Where the box's packages come from (docs/ADMINHUB_PLAN.md, "Feeds"): each
  source can be switched on or off, and a new one added. Both go through
  preview-and-confirm; adding shows a standing warning, since a source's
  packages are installed with root rights and these feeds are unsigned.
  Collapsed by default — most people never need this.
-->
<template>
  <details class="mt-5 rounded border border-line bg-tile p-4" :open="open" @toggle="open = ($event.target as HTMLDetailsElement).open">
    <summary class="cursor-pointer text-[14px] font-semibold text-text">
      {{ t('admin.packages.feedsTitle') }}
    </summary>
    <p class="mt-1 text-[12.5px] text-muted">
      {{ t('admin.packages.feedsIntro') }}
    </p>
    <p v-if="error" class="mt-2 text-[13px] text-faint">{{ error }}</p>
    <ul v-else class="mt-2 flex flex-col">
      <li v-for="f in feeds" :key="f.name" class="flex items-center gap-3 border-b border-line-soft py-2 last:border-none">
        <div class="min-w-0 flex-1">
          <span class="block font-mono text-[13px] text-text">{{ f.name }}</span>
          <span v-if="f.url" class="block truncate text-[12px] text-faint">{{ f.url }}</span>
        </div>
        <span v-if="!f.known" class="shrink-0 rounded border border-warn px-1.5 py-0.5 text-[11px] text-warn">{{ t('admin.packages.feedAddedByHand') }}</span>
        <button
          type="button"
          role="switch"
          :aria-checked="f.enabled"
          :aria-label="f.name"
          class="relative h-5 w-9 shrink-0 rounded-full transition-colors disabled:opacity-60"
          :class="f.enabled ? 'bg-accent' : 'bg-tile-2'"
          :disabled="!!pending"
          @click="previewSwitch(f)"
        >
          <span class="absolute top-0.5 h-4 w-4 rounded-full bg-bg transition-all" :class="f.enabled ? 'left-[18px]' : 'left-0.5'" />
        </button>
      </li>
    </ul>

    <form v-if="!adding && !pending" class="mt-3" @submit.prevent>
      <button type="button" class="text-[13px] text-accent hover:underline" @click="adding = true">
        {{ t('admin.packages.feedAdd') }}
      </button>
    </form>
    <form v-else-if="adding && !pending" class="mt-3 flex flex-col gap-2" @submit.prevent="previewAdd">
      <input
        v-model="newName"
        type="text"
        required
        maxlength="60"
        :placeholder="t('admin.packages.feedName')"
        class="rounded border border-line bg-field px-2.5 py-1.5 font-mono text-[13px] text-text"
      />
      <input
        v-model="newUrl"
        type="url"
        required
        :placeholder="t('admin.packages.feedUrl')"
        class="rounded border border-line bg-field px-2.5 py-1.5 font-mono text-[13px] text-text"
      />
      <p class="text-[12px] text-faint">{{ t('admin.packages.feedHint') }}</p>
      <div class="flex justify-end gap-2">
        <button type="button" class="rounded px-3 py-1.5 text-[13px] text-text hover:bg-field" @click="adding = false">
          {{ t('plan.cancel') }}
        </button>
        <button type="submit" class="rounded bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg">
          {{ t('admin.packages.feedPreview') }}
        </button>
      </div>
    </form>

    <div v-if="pending" class="mt-3">
      <p v-if="!pending.plan && !pending.error" class="text-[13px] text-faint">
        {{ t('common.loading') }}
      </p>
      <PlanPreview
        v-else-if="pending.plan"
        :plan="pending.plan"
        :confirm-label="t('admin.packages.confirm')"
        :busy="pending.applying"
        :error="pending.error"
        @confirm="apply"
        @cancel="pending = null"
      >
        {{ pending.plan.summary }}
      </PlanPreview>
      <div v-else class="flex items-start justify-between gap-3">
        <p class="text-[12.5px] text-warn">{{ pending.error }}</p>
        <button type="button" class="shrink-0 text-[12.5px] text-accent hover:underline" @click="pending = null">
          {{ t('extensions.settings.close') }}
        </button>
      </div>
    </div>
  </details>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import PlanPreview from '@/components/PlanPreview.vue'
import { useFriendlyError } from '@/composables/useFriendlyError'
import { usePlanError } from '@/composables/usePlanError'
import type { AdminFeed, Plan } from '@/api/types'

const emit = defineEmits<{ changed: [] }>()
const { t } = useI18n()
const { friendly } = useFriendlyError()
const { planErrorMessage } = usePlanError()

const feeds = ref<AdminFeed[]>([])
const error = ref('')
const open = ref(false)
const adding = ref(false)
const newName = ref('')
const newUrl = ref('')
const pending = ref<{ plan?: Plan; applying?: boolean; error?: string } | null>(null)

async function load() {
  try {
    feeds.value = (await api.admin.feeds()).feeds
    error.value = ''
  } catch (err) {
    error.value = friendly(err).text
  }
}

async function plan(action: 'enable' | 'disable' | 'add', name: string, url?: string) {
  pending.value = {}
  try {
    pending.value = {
      plan: (await api.admin.feedPlan(action, name, url)).plan,
    }
  } catch (err) {
    pending.value = { error: planErrorMessage(err) }
  }
}

const previewSwitch = (f: AdminFeed) => plan(f.enabled ? 'disable' : 'enable', f.name)
const previewAdd = () => plan('add', newName.value.trim(), newUrl.value.trim())

async function apply() {
  const p = pending.value
  if (!p?.plan) return
  pending.value = { ...p, applying: true, error: undefined }
  try {
    await api.plans.apply(p.plan.id)
    pending.value = null
    adding.value = false
    newName.value = newUrl.value = ''
    await load()
    emit('changed') // the package list follows what the feeds offer
  } catch (err) {
    pending.value = { plan: p.plan, error: planErrorMessage(err) }
  }
}

onMounted(load)
</script>
