<!--
  "Clean up unused images" (docs/APPSHUB_PLAN.md, App13): the size comes
  first, then a plan with the list of what goes, then the confirm. The
  helper does it with two fixed calls; nothing is chosen image by image.
-->
<template>
  <section class="mt-5 border-t border-line-soft pt-4">
    <h3 class="text-[13px] font-semibold text-muted">{{ t('apps.images.title') }}</h3>
    <p class="mt-1 text-[12.5px] text-faint">{{ t('apps.images.hint') }}</p>
    <p v-if="error" class="mt-2 text-[12.5px] text-warn" role="alert">{{ error }}</p>
    <p v-else-if="!info" class="mt-2 text-[13px] text-faint">{{ t('common.loading') }}</p>
    <template v-else-if="!pending">
      <p v-if="info.images.length === 0" class="mt-2 text-[13px] text-text">{{ t('apps.images.none') }}</p>
      <div v-else class="mt-2 flex flex-wrap items-center gap-3">
        <span class="text-[13px] text-text">{{ t('apps.images.reclaimable', { size: formatBytes(info.reclaimableBytes, locale), count: info.images.length }) }}</span>
        <button type="button" class="rounded-sm border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field disabled:opacity-60" :disabled="busy" @click="preview">{{ t('apps.images.button') }}</button>
      </div>
      <p v-if="done" class="mt-2 flex items-center gap-1.5 text-[12.5px] text-ok" role="status"><span class="h-1.5 w-1.5 rounded-full bg-ok" />{{ done }}</p>
    </template>
    <div v-if="pending" class="mt-3">
      <PlanPreview :plan="pending" :confirm-label="t('apps.images.confirm')" :busy="applying" :error="planError" @confirm="confirm" @cancel="pending = null">
        {{ t('apps.images.summary', { size: formatBytes(info?.reclaimableBytes ?? 0, locale), count: info?.images.length ?? 0 }) }}
        <ul class="mt-2 flex flex-col gap-0.5 text-[12.5px] text-muted">
          <li v-for="i in info?.images ?? []" :key="i.id" class="flex justify-between gap-3">
            <span class="min-w-0 break-all">{{ i.tags.length ? i.tags.join(', ') : t('apps.images.untagged') }}</span>
            <span class="shrink-0 text-faint">{{ formatBytes(i.sizeBytes, locale) }}</span>
          </li>
        </ul>
      </PlanPreview>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import PlanPreview from '@/components/PlanPreview.vue'
import { usePlanError } from '@/composables/usePlanError'
import { formatBytes } from '@/lib/format'
import type { Plan, UnusedImages } from '@/api/types'

const { t, locale } = useI18n()
const { planErrorMessage } = usePlanError()

const info = ref<UnusedImages | null>(null)
const error = ref('')
const pending = ref<Plan | null>(null)
const planError = ref('')
const applying = ref(false)
const busy = ref(false)
const done = ref('')

async function load() {
  try {
    info.value = await api.apps.unusedImages()
  } catch {
    error.value = t('apps.images.loadError')
  }
}

async function preview() {
  busy.value = true
  planError.value = ''
  done.value = ''
  try {
    pending.value = (await api.apps.pruneImagesPlan()).plan
  } catch (err) {
    error.value = planErrorMessage(err)
  } finally {
    busy.value = false
  }
}

async function confirm() {
  if (!pending.value) return
  applying.value = true
  planError.value = ''
  const freed = info.value?.reclaimableBytes ?? 0
  try {
    await api.plans.apply(pending.value.id)
    pending.value = null
    done.value = t('apps.images.done', { size: formatBytes(freed, locale.value) })
    await load()
  } catch (err) {
    planError.value = planErrorMessage(err)
  } finally {
    applying.value = false
  }
}

onMounted(load)
</script>
