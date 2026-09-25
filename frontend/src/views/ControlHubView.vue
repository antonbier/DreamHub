<!--
  ControlHub (CONCEPT.md: "Web remote, message to TV, screenshot, power"),
  pulled forward from v1.1 into v1 (docs/V1_PLAN.md) — verifying WP8's
  Enigma2 plugin needed a real screenshot/remote-control loop anyway, and
  GuideHub's "switch to this channel now" needs it too. Every action here
  is direct and immediate, not preview-and-confirm (PRINCIPLES.md): a
  virtual remote is inherently a live, instantly-reversible input device,
  the same way a physical remote (or WebControl's own native remote page,
  which DreamHub deliberately doesn't reimplement) works — see
  backend/internal/hubs/control.go's own doc comment.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="control" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ t(item ? item.label : 'hub.control') }}</h1>
    </div>

    <div v-if="item && item.status && item.status !== 'available'" class="mt-4 rounded border border-line bg-tile p-4">
      <div class="mb-1.5 flex items-center gap-2 text-[13.5px] font-semibold">
        <span class="h-1.5 w-1.5 rounded-full" :class="item.status === 'degraded' ? 'bg-warn' : 'bg-faint'" />
        {{ t(`status.${item.status}`) }}
      </div>
      <ul class="flex flex-col gap-1 text-[13px] text-muted">
        <li v-for="(p, i) in item.problems ?? []" :key="i">
          {{ describe(p) }}
          <span v-if="describeFix(p)" class="text-faint">{{ ' — ' + describeFix(p) }}</span>
        </li>
      </ul>
    </div>

    <template v-if="!item || item.status !== 'unavailable'">
      <div v-if="inStandby" class="mt-5 flex flex-wrap items-center justify-between gap-3 rounded border border-line bg-tile p-4">
        <span class="text-[13.5px] text-text">{{ t('control.standby.banner') }}</span>
        <button type="button" class="shrink-0 rounded bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg" @click="wake">
          {{ t('control.standby.wake') }}
        </button>
      </div>

      <div class="mt-5 grid grid-cols-1 gap-5 lg:grid-cols-[1fr_auto]">
        <!-- Screenshot -->
        <section class="rounded border border-line bg-tile p-3">
          <div class="mb-2 flex items-center justify-between gap-3">
            <h2 class="text-[14px] font-semibold text-text">{{ t('control.screenshot.title') }}</h2>
            <div class="flex items-center gap-2">
              <label class="flex items-center gap-1.5 text-[12.5px] text-muted">
                <input v-model="autoRefresh" type="checkbox" class="rounded" />
                {{ t('control.screenshot.auto') }}
              </label>
              <button type="button" class="rounded border border-line px-2.5 py-1 text-[12.5px] text-text hover:bg-field" @click="refreshShot">
                {{ t('control.screenshot.refresh') }}
              </button>
            </div>
          </div>
          <div class="flex aspect-video items-center justify-center overflow-hidden rounded bg-bg">
            <img v-if="shotSrc" :src="shotSrc" :alt="t('control.screenshot.title')" class="max-h-full max-w-full" @error="shotFailed = true" />
            <p v-else class="text-[13px] text-faint">{{ shotFailed ? t('error.plan.webcontrol_unavailable') : t('common.loading') }}</p>
          </div>
        </section>

        <!-- Remote pad -->
        <section class="rounded border border-line bg-tile p-4">
          <h2 class="mb-3 text-[14px] font-semibold text-text">{{ t('control.pad.title') }}</h2>
          <div class="flex flex-col items-center gap-2">
            <RemoteButton :label="t('control.keys.menu')" @click="key(KEY.MENU)" />
            <div class="grid grid-cols-3 gap-2">
              <span />
              <RemoteButton :label="t('control.keys.up')" @click="key(KEY.UP)" />
              <span />
              <RemoteButton :label="t('control.keys.left')" @click="key(KEY.LEFT)" />
              <RemoteButton :label="t('control.keys.ok')" primary @click="key(KEY.OK)" />
              <RemoteButton :label="t('control.keys.right')" @click="key(KEY.RIGHT)" />
              <span />
              <RemoteButton :label="t('control.keys.down')" @click="key(KEY.DOWN)" />
              <span />
            </div>
            <RemoteButton :label="t('control.keys.exit')" @click="key(KEY.EXIT)" />
            <div class="mt-2 grid grid-cols-4 gap-2">
              <RemoteButton :label="t('control.keys.red')" color="bg-onair" @click="key(KEY.RED)" />
              <RemoteButton :label="t('control.keys.green')" color="bg-ok" @click="key(KEY.GREEN)" />
              <RemoteButton :label="t('control.keys.yellow')" color="bg-warn" @click="key(KEY.YELLOW)" />
              <RemoteButton :label="t('control.keys.blue')" color="bg-accent" @click="key(KEY.BLUE)" />
            </div>
            <RemoteButton :label="t('control.keys.power')" class="mt-2" @click="key(KEY.POWER)" />
          </div>
        </section>
      </div>
      <p v-if="keyError" class="mt-3 text-[12.5px] text-warn">{{ keyError }}</p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import Icon from '@/components/Icon.vue'
import { useNavigationStore } from '@/stores/navigation'
import { useProblem } from '@/composables/useProblem'
import { usePlanError } from '@/composables/usePlanError'

const { t } = useI18n()
const navigation = useNavigationStore()
const { describe, describeFix } = useProblem()
const { planErrorMessage } = usePlanError()

const item = computed(() => navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.path === '/control'))

// Standard Linux input-event codes, confirmed live against the reference
// box (ARCHITECTURE.md, "Real screenshot and remote-control endpoints").
const KEY = {
  UP: 103,
  DOWN: 108,
  LEFT: 105,
  RIGHT: 106,
  OK: 352,
  EXIT: 174,
  MENU: 139,
  RED: 398,
  GREEN: 399,
  YELLOW: 400,
  BLUE: 401,
  POWER: 116,
} as const

const shotSrc = ref('')
const shotFailed = ref(false)
function refreshShot() {
  shotFailed.value = false
  shotSrc.value = api.control.screenshotPath()
}

const autoRefresh = ref(false)
let autoTimer: ReturnType<typeof setInterval> | undefined
watch(autoRefresh, (on) => {
  if (autoTimer) clearInterval(autoTimer)
  autoTimer = on ? setInterval(refreshShot, 4000) : undefined
})
onUnmounted(() => {
  if (autoTimer) clearInterval(autoTimer)
})

const inStandby = ref(false)
async function loadPower() {
  try {
    inStandby.value = (await api.control.power()).inStandby
  } catch {
    // Leave the last known state rather than flashing a wrong banner.
  }
}

const keyError = ref('')
async function key(code: number) {
  keyError.value = ''
  try {
    await api.control.sendKey(code)
    if (code === KEY.POWER) await loadPower()
  } catch (err) {
    keyError.value = planErrorMessage(err)
  }
}

async function wake() {
  await key(KEY.POWER)
}

onMounted(() => {
  refreshShot()
  void loadPower()
})

// A small local component for the pad's buttons — not worth its own file
// for one screen's use.
const RemoteButton = defineComponent({
  props: { label: String, primary: Boolean, color: String },
  emits: ['click'],
  setup(props, { emit }) {
    return () =>
      h(
        'button',
        {
          type: 'button',
          'aria-label': props.label,
          class: [
            'flex h-11 w-11 items-center justify-center rounded-full text-[11px] font-semibold',
            props.primary ? 'bg-accent text-bg' : props.color ? props.color + ' text-bg' : 'border border-line text-text hover:bg-field',
          ],
          onClick: () => emit('click'),
        },
        props.label?.slice(0, 2),
      )
  },
})
</script>
