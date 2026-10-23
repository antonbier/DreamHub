<!--
  One setting of the receiver's setup menus (docs/BOXSETTINGS_PLAN.md, S1;
  read only): the box's own label, its current value, the path and the
  level in a small line below, and the marks — a risky setting, one that
  needs a GUI restart. A "?" button opens a short explanation under the row:
  DreamHub's own text for the setting if there is one, the receiver's own
  description (English) otherwise, plus what the marks mean. A setting with
  nothing to explain has no button — its label says it all.
-->
<template>
  <li class="border-b border-line-soft py-2.5 last:border-none">
    <div class="flex items-start gap-3">
      <div class="min-w-0 flex-1">
        <p class="text-[14px] text-text">{{ item.label }}</p>
        <p class="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-[12px] text-faint">
          <span class="break-all font-mono">{{ item.path }}</span>
          <span v-if="item.levelKnown && item.level > 0">{{ t(`boxsettings.levelName.${item.level}`) }}</span>
          <span v-if="item.risk" class="inline-flex items-center gap-1 text-warn">
            <Icon name="warning" :size="13" />
            {{ t(`boxsettings.risk.${item.risk}`) }}
          </span>
          <span v-if="item.guiRestart" class="text-muted">{{ t('boxsettings.guiRestart') }}</span>
        </p>
      </div>
      <p class="max-w-[45%] shrink-0 break-words text-end text-[13.5px] text-text below-sm:max-w-[40%]" :class="{ 'text-muted': item.kind === 'readonly' }">
        {{ shown }}
      </p>
      <button
        v-if="hasHelp"
        type="button"
        class="grid size-7 shrink-0 place-items-center rounded-full text-muted hover:bg-field hover:text-text"
        :aria-expanded="open"
        :aria-controls="helpId"
        :aria-label="t('boxsettings.helpButton', { name: item.label })"
        :title="t('boxsettings.helpTitle')"
        @click="open = !open"
      >
        <Icon name="help" :size="18" />
      </button>
      <span v-else class="size-7 shrink-0" aria-hidden="true" />
    </div>
    <div v-if="hasHelp && open" :id="helpId" class="mt-2 rounded-sm border border-line-soft bg-field p-3 text-[13px] leading-relaxed text-text">
      <p v-if="ownHelp">{{ ownHelp }}</p>
      <p v-if="item.help" :class="ownHelp ? 'mt-2 text-muted' : ''">
        <span v-if="ownHelp" class="text-faint">{{ t('boxsettings.fromBox') }}: </span>{{ item.help }}
      </p>
      <p v-if="item.risk && item.riskCategory" class="mt-2 text-warn">
        <strong class="font-semibold">{{ t(`boxsettings.risk.${item.risk}`) }}.</strong>
        {{ t(`boxsettings.riskWhy.${item.riskCategory}`) }}
      </p>
      <p v-if="item.guiRestart" class="mt-2 text-muted">{{ t('boxsettings.guiRestartWhy') }}</p>
      <p v-if="item.secret" class="mt-2 text-muted">{{ t('boxsettings.secretWhy') }}</p>
    </div>
  </li>
</template>

<script setup lang="ts">
import { computed, ref, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/Icon.vue'
import { formatValue, helpKey } from '@/lib/boxSettings'
import type { BoxSetting } from '@/api/types'

const props = defineProps<{ item: BoxSetting }>()
const { t, te, locale } = useI18n()

const open = ref(false)
const helpId = `bs-help-${useId()}`

const shown = computed(() => formatValue(props.item, t, locale.value))

// DreamHub's own explanation of this setting, when it has one.
const ownHelp = computed(() => {
  const key = `boxsettings.help.${helpKey(props.item.path)}`
  return te(key) ? t(key) : ''
})

const hasHelp = computed(() => !!(ownHelp.value || props.item.help || props.item.risk || props.item.guiRestart || props.item.secret))
</script>
