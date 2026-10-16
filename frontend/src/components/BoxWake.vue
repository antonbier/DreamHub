<!--
  "Switch on" for a box that does not answer (docs/MULTIDEVICE_PLAN.md, M9) —
  only where a way is set up (Settings → Boxes) and the profile has the
  remote. Wake-on-LAN goes out at once; an extension's action (the smart
  plug the box hangs on) is previewed first with the extension's own
  sentence, because a socket may toggle. Afterwards the app looks at the box
  every few seconds until it answers.
-->
<template>
  <span v-if="visible" class="inline-flex items-center gap-2">
    <span v-if="starting" class="text-[12.5px] text-muted" role="status">{{ t('boxes.wake.starting') }}</span>
    <button
      v-else
      type="button"
      class="rounded-sm border border-line px-2 py-0.5 text-[12.5px] text-text hover:bg-tile-2 disabled:opacity-60"
      :disabled="busy"
      @click.stop="start"
    >
      {{ t('boxes.wake.button') }}
    </button>
    <span v-if="error" class="text-[12.5px] text-warn" role="alert">{{ error }}</span>
  </span>
  <Teleport to="body">
    <div v-if="plan" class="fixed inset-x-4 top-24 z-50 mx-auto max-w-md shadow-lg">
      <PlanPreview :plan="plan" :confirm-label="t('boxes.wake.confirm')" :busy="busy" :error="error" @confirm="confirm" @cancel="plan = null">
        {{ plan.summary }}
      </PlanPreview>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import { useAuth } from '@/composables/useAuth'
import { usePlanError } from '@/composables/usePlanError'
import PlanPreview from '@/components/PlanPreview.vue'
import { useBoxesStore } from '@/stores/boxes'
import { boxErrorKey } from '@/components/settings/boxErrors'
import type { Plan } from '@/api/types'

const props = defineProps<{ boxId: string }>()
const { t } = useI18n()
const boxes = useBoxesStore()
const { planErrorMessage } = usePlanError()

const box = computed(() => boxes.boxes.find((b) => b.id === props.boxId))
const starting = computed(() => boxes.waking.includes(props.boxId))
const visible = computed(() => !!box.value?.wake && (!box.value.online || starting.value) && useAuth().can('control'))

const busy = ref(false)
const error = ref('')
const plan = ref<Plan | null>(null)

async function start() {
  busy.value = true
  error.value = ''
  try {
    const res = await api.boxes.wake(props.boxId)
    if (res.plan) plan.value = res.plan
    else boxes.watchWake(props.boxId)
  } catch (err) {
    error.value = t(boxErrorKey(err))
  } finally {
    busy.value = false
  }
}

async function confirm() {
  if (!plan.value) return
  busy.value = true
  error.value = ''
  try {
    await api.plans.apply(plan.value.id)
    plan.value = null
    boxes.watchWake(props.boxId)
  } catch (err) {
    error.value = planErrorMessage(err)
  } finally {
    busy.value = false
  }
}
</script>
