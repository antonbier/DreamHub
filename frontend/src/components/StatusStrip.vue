<!--
  Status line under the top bar (DESIGN.md, "Status line": "short items
  with a colored dot, not pills... the display time zone right-aligned in
  faint"). Replaces the earlier ad hoc "backend unreachable" banner — the
  same information now lives here as the first dot, consistently with
  everything else on the line.
-->
<template>
  <div class="flex h-8 shrink-0 items-center gap-4 overflow-hidden whitespace-nowrap border-b border-line bg-bg px-4 text-[12.5px] below-sm:gap-3">
    <span class="flex items-center gap-1.5" :class="status.reachable.value ? 'text-muted' : 'text-warn'">
      <span class="h-1.5 w-1.5 rounded-full" :class="status.reachable.value ? 'bg-ok' : 'bg-warn'" />
      {{ status.reachable.value ? t('statusStrip.connected') : t('statusStrip.unreachable') }}
    </span>

    <span v-if="status.clockSkew.value !== null" class="flex items-center gap-1.5" :class="status.clockOk.value ? 'text-muted' : 'text-warn'">
      <span class="h-1.5 w-1.5 rounded-full" :class="status.clockOk.value ? 'bg-ok' : 'bg-warn'" />
      {{ status.clockOk.value ? t('statusStrip.clockOk') : t('statusStrip.clockOff', { minutes: status.clockSkew.value }) }}
    </span>

    <span v-if="status.issueCount.value > 0" class="flex items-center gap-1.5 text-warn">
      <span class="h-1.5 w-1.5 rounded-full bg-warn" />
      {{ status.issueCount.value === 1 ? t('statusStrip.issueSingular') : t('statusStrip.issuePlural', { count: status.issueCount.value }) }}
    </span>

    <span class="ms-auto truncate text-faint below-sm:hidden">{{ timeZone }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDeviceStatus } from '@/composables/useDeviceStatus'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'

const { t } = useI18n()
const status = useDeviceStatus()
const { timeZone: displayTimeZone } = useDisplayTimeZone()
onMounted(status.checkClock)

// The effective display zone (general.timezoneMode/general.timezone) when
// resolved, falling back to the browser's own — resolvedOptions() always
// returns a concrete IANA name even when Intl's `timeZone` option was
// undefined, so this line is honest either way.
const timeZone = computed(() => Intl.DateTimeFormat(undefined, { timeZone: displayTimeZone.value }).resolvedOptions().timeZone)
</script>
