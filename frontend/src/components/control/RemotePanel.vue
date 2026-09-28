<!--
  The remote as a side panel any page can open (top bar button), so the TV
  can be steered while looking at the guide, a timer or a recording. Docked
  at the inline end next to the page on desktop; an overlay drawer below
  900px (DESIGN.md's drawer breakpoint). Not shown on ControlHub's own
  page, which has the full-size remote already. Escape closes it and hands
  focus back to the button that opened it.
-->
<template>
  <template v-if="shown">
    <div class="fixed inset-0 z-20 hidden bg-black/50 below-md:block" @click="close" />
    <aside
      id="remote-panel"
      :aria-label="t('control.pad.title')"
      class="z-30 flex w-[320px] shrink-0 flex-col border-s border-line bg-side below-md:fixed below-md:inset-y-0 below-md:end-0 below-md:w-[min(340px,100%)]"
      @keydown.esc.stop="close"
    >
      <div class="flex h-14 shrink-0 items-center gap-2 border-b border-line px-3">
        <Icon name="control" :size="18" class="text-muted" />
        <h2 class="flex-1 truncate text-[14px] font-semibold">{{ t('control.pad.title') }}</h2>
        <RouterLink to="/control" class="rounded px-2 py-1 text-[12.5px] text-accent hover:bg-tile-2">
          {{ t('control.panel.openPage') }}
        </RouterLink>
        <button
          type="button"
          class="flex h-8 w-8 items-center justify-center rounded text-muted hover:bg-tile-2 hover:text-text"
          :aria-label="t('control.panel.close')"
          @click="close"
        >
          <Icon name="close" :size="18" />
        </button>
      </div>
      <div class="flex flex-1 flex-col gap-4 overflow-y-auto p-4">
        <ScreenView />
        <RemotePad ref="pad" compact />
      </div>
    </aside>
  </template>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/Icon.vue'
import RemotePad from './RemotePad.vue'
import ScreenView from './ScreenView.vue'
import { useRemoteStore } from '@/stores/remote'
import { useControlHub } from '@/composables/useControlHub'

const { t } = useI18n()
const route = useRoute()
const remote = useRemoteStore()
const { available } = useControlHub()

const shown = computed(() => remote.panelOpen && available.value && route.path !== '/control')
const pad = ref<InstanceType<typeof RemotePad> | null>(null)

// Opening the panel puts keyboard focus on the pad, so the arrow keys
// steer the box right away. Only on the user's own toggle — a panel that
// was remembered open doesn't grab focus on page load.
watch(() => remote.panelOpen, async (open) => {
  if (open) {
    await nextTick()
    pad.value?.focus()
  }
})

function close() {
  remote.setPanelOpen(false)
  void nextTick(() => document.getElementById('remote-panel-toggle')?.focus())
}
</script>
