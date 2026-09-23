// Drives the settings menu (src/views/SettingsView.vue) against
// GET/PATCH /api/v1/settings. One save per section, matching the schema's
// cross-field checks, which are scoped per section (backend/internal/settings,
// Section.Check) — see backend/README.md's API table.
//
// DreamHub's own settings save directly, no preview-and-confirm (PRINCIPLES.md
// names this exception) — the backend still records every change in the
// change log.
import { reactive, ref } from 'vue'
import { api, ApiError } from '@/api/client'
import type { FieldError, SettingSection } from '@/api/types'

export function useSettingsForm() {
  const sections = ref<SettingSection[]>([])
  // Keyed by field key. Password fields never come back from the API
  // (write-only), so their draft/original both start as undefined —
  // "nothing typed" is distinct from "clear the existing password".
  const drafts = reactive<Record<string, unknown>>({})
  const original = reactive<Record<string, unknown>>({})
  const errors = reactive<Record<string, FieldError>>({})
  const savingSection = ref<string | null>(null)
  const loading = ref(true)
  const loadFailed = ref(false)

  function applyView(view: { sections: SettingSection[] }) {
    sections.value = view.sections
    for (const sec of view.sections) {
      for (const f of sec.fields) {
        const v = f.type === 'password' ? undefined : f.value
        drafts[f.key] = v
        original[f.key] = v
      }
    }
  }

  async function load() {
    loading.value = true
    loadFailed.value = false
    try {
      applyView(await api.settings.get())
    } catch {
      loadFailed.value = true
    } finally {
      loading.value = false
    }
  }

  function isDirty(key: string): boolean {
    return drafts[key] !== original[key]
  }
  function sectionDirty(section: SettingSection): boolean {
    return section.fields.some((f) => isDirty(f.key))
  }

  function setValue(key: string, value: unknown) {
    drafts[key] = value
    delete errors[key]
  }

  // null tells the backend to reset to default — for a secret/password
  // field, its "default" is simply cleared (backend/README.md).
  function resetField(key: string) {
    setValue(key, null)
  }

  // Returns the just-saved section as confirmed by the server (or
  // undefined if there was nothing to save), so a caller that needs the
  // authoritative new values right away (see SettingsSectionCard.vue
  // syncing the preferences store) doesn't have to wait a tick for props
  // to catch up with `sections`.
  async function saveSection(section: SettingSection): Promise<SettingSection | undefined> {
    const changed = section.fields.filter((f) => isDirty(f.key))
    if (changed.length === 0) return undefined
    savingSection.value = section.id
    const values: Record<string, unknown> = {}
    for (const f of changed) values[f.key] = drafts[f.key]
    try {
      const view = await api.settings.update(values)
      applyView(view)
      for (const f of changed) delete errors[f.key]
      return view.sections.find((s) => s.id === section.id)
    } catch (err) {
      if (err instanceof ApiError && err.details) {
        for (const [key, fe] of Object.entries(err.details)) errors[key] = fe
      }
      throw err
    } finally {
      savingSection.value = null
    }
  }

  function cancelSection(section: SettingSection) {
    for (const f of section.fields) {
      drafts[f.key] = original[f.key]
      delete errors[f.key]
    }
  }

  return {
    sections,
    drafts,
    errors,
    loading,
    loadFailed,
    savingSection,
    load,
    isDirty,
    sectionDirty,
    setValue,
    resetField,
    saveSection,
    cancelSection,
  }
}
