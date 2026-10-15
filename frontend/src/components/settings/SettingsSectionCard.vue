<!--
  One settings section as a tile: title, its fields, and a Save/Cancel
  footer that only appears once something in the section changed
  (DESIGN.md: "At most one filled primary button per tile").
-->
<template>
  <section class="rounded border border-line bg-tile p-5">
    <div class="mb-4 flex items-center gap-3">
      <h2 class="text-[15.5px] font-semibold">{{ sectionTitle(section) }}</h2>
      <!-- A connector's state, on the services tab. -->
      <span v-if="status" class="ms-auto flex shrink-0 items-center gap-1.5 text-[12.5px]" :class="statusClass">
        <span class="h-1.5 w-1.5 rounded-full" :class="statusDot" />
        {{ t(`settings.connectorState.${status}`) }}
      </span>
    </div>
    <div class="flex flex-col gap-4">
      <SettingsFieldInput
        v-for="field in visibleFields"
        :key="field.key"
        :field="field"
        :model-value="form.drafts[field.key]"
        :original="originalOf(field.key)"
        :error="form.errors[field.key]"
        @update:model-value="form.setValue(field.key, $event)"
        @reset="form.resetField(field.key)"
      />
    </div>
    <div v-if="form.sectionDirty(section)" class="mt-5 flex items-center gap-3 border-t border-line-soft pt-4">
      <button
        type="button"
        class="rounded bg-accent px-3.5 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-60"
        :disabled="form.savingSection.value === section.id"
        @click="save"
      >
        {{ form.savingSection.value === section.id ? t('settings.page.saving') : t('settings.page.save') }}
      </button>
      <button
        type="button"
        class="text-[13px] text-muted hover:text-text"
        :disabled="form.savingSection.value === section.id"
        @click="form.cancelSection(section)"
      >
        {{ t('settings.page.cancel') }}
      </button>
      <span class="text-[12.5px] text-faint">{{ t('settings.page.unsavedChanges') }}</span>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import SettingsFieldInput from './SettingsFieldInput.vue'
import { useSettingsText } from '@/composables/useSettingsText'
import type { useSettingsForm } from '@/composables/useSettingsForm'
import { usePreferencesStore } from '@/stores/preferences'
import { useNavigationStore } from '@/stores/navigation'
import { isSupportedLocale } from '@/i18n'
import type { ConnectorState, SettingSection } from '@/api/types'

const props = defineProps<{
  section: SettingSection
  form: ReturnType<typeof useSettingsForm>
  // The section's connector state (services tab); absent elsewhere.
  status?: ConnectorState['state']
}>()

const { t } = useI18n()
const { sectionTitle } = useSettingsText()
const preferences = usePreferencesStore()
const navigation = useNavigationStore()

const statusClass = computed(() => (props.status === 'unreachable' ? 'text-warn' : 'text-muted'))
const statusDot = computed(() => {
  switch (props.status) {
    case 'ok':
      return 'bg-ok'
    case 'unreachable':
      return 'bg-warn'
    default:
      return 'bg-faint'
  }
})

// general.timezone only matters in "fixed" mode — hidden otherwise rather
// than shown disabled, since it has nothing to say the rest of the time.
const visibleFields = computed(() =>
  props.section.fields.filter(
    (f) => !f.hidden && (f.key !== 'general.timezone' || props.form.drafts['general.timezoneMode'] === 'fixed'),
  ),
)

function originalOf(key: string) {
  // useSettingsForm's `original` is a plain reactive object, not exposed
  // directly — re-derive it from the section field's last-loaded value via
  // the form's own bookkeeping instead of poking at internals.
  const field = props.section.fields.find((f) => f.key === key)
  return field?.type === 'password' ? undefined : field?.value
}

async function save() {
  try {
    const saved = await props.form.saveSection(props.section)
    if (saved) {
      syncPreferences(saved)
      // Any section can affect the sidebar: a hub's own enable switch
      // (the "features" section), or a connector's settings a hub requires to
      // be available (e.g. the "ai"/"tmdb" sections) — so refresh it after
      // every successful save, not just this one section's own fields.
      void navigation.load()
    }
  } catch {
    // Field-level errors are already in form.errors and shown inline;
    // nothing else to do here.
  }
}

// The general section overlaps with what the top-bar/user-menu already
// manage live (preferences.ts): saving language/appearance here
// must update that reactive state too, or the UI would only pick up the
// change after a reload despite the backend already having it.
function syncPreferences(saved: SettingSection) {
  if (saved.id !== 'general') return
  const value = (key: string) => saved.fields.find((f) => f.key === key)?.value

  const lang = value('general.language')
  if (typeof lang === 'string' && isSupportedLocale(lang)) preferences.setLanguage(lang, { persist: false })

  const appearance = value('general.appearance')
  if (appearance === 'dark' || appearance === 'light' || appearance === 'system') {
    preferences.setAppearance(appearance, { persist: false })
  }

  const timezoneMode = value('general.timezoneMode')
  if (timezoneMode === 'box' || timezoneMode === 'browser' || timezoneMode === 'fixed') preferences.timezoneMode = timezoneMode
  const timezone = value('general.timezone')
  if (typeof timezone === 'string') preferences.timezone = timezone
}
</script>
