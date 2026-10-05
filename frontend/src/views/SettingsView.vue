<!--
  The central settings menu (CONCEPT.md, "Settings"): rendered straight from
  GET /api/v1/settings — every section and field the backend registers
  (general, auth, network, each connector, hubs) shows up here with no
  frontend-side list to keep in sync. See src/composables/useSettingsForm.ts.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="mb-4 flex items-center">
      <h1 class="text-[26px] font-bold">{{ t('settings.page.title') }}</h1>
      <RouterLink to="/onboarding" class="ms-auto text-[13.5px] text-accent hover:underline">
        {{ t('settings.page.runWizard') }}
      </RouterLink>
    </div>

    <p v-if="form.loading.value" class="text-[14px] text-muted">{{ t('common.loading') }}</p>
    <div v-else-if="form.loadFailed.value" class="rounded border border-line bg-tile p-5">
      <p class="mb-3 text-[14px] text-muted">{{ t('settings.page.loadFailed') }}</p>
      <button type="button" class="text-[13.5px] text-accent hover:underline" @click="form.load()">
        {{ t('common.retry') }}
      </button>
    </div>
    <div v-else class="flex flex-col gap-4">
      <!-- Twelve sections make a long page: a row of jump links, sticky at
           the top, scrollable sideways on a phone. /settings#tmdb works too. -->
      <nav
        class="sticky top-0 z-10 -mx-6 -mt-2 flex gap-1 overflow-x-auto border-b border-line-soft bg-bg/95 px-6 py-2 backdrop-blur below-md:-mx-4 below-md:px-4"
        :aria-label="t('settings.page.jumpTo')"
      >
        <a
          v-for="section in form.sections.value"
          :key="section.id"
          :href="`#${section.id}`"
          class="shrink-0 whitespace-nowrap rounded px-2.5 py-1 text-[13px]"
          :class="current === section.id ? 'bg-tile-2 text-text' : 'text-muted hover:bg-tile-2 hover:text-text'"
          @click.prevent="jump(section.id)"
        >
          {{ text.sectionTitle(section) }}
        </a>
      </nav>
      <template v-for="section in form.sections.value" :key="section.id">
        <SettingsSectionCard :id="section.id" :section="section" :form="form" class="scroll-mt-14" />
        <!-- Not a schema field — tokens are a list of their own resources,
             not a single setting — so this is placed by hand right after
             Authentication rather than coming from the section loop. -->
        <ApiTokensPanel v-if="section.id === 'auth'" />
        <PushPanel v-if="section.id === 'push'" />
        <InstallPanel v-if="section.id === 'general'" />
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import SettingsSectionCard from '@/components/settings/SettingsSectionCard.vue'
import ApiTokensPanel from '@/components/settings/ApiTokensPanel.vue'
import PushPanel from '@/components/settings/PushPanel.vue'
import InstallPanel from '@/components/settings/InstallPanel.vue'
import { useSettingsForm } from '@/composables/useSettingsForm'
import { useSettingsText } from '@/composables/useSettingsText'

const { t } = useI18n()
const text = useSettingsText()
const form = useSettingsForm()
const route = useRoute()
const router = useRouter()

// The section at the top of the scroll area, highlighted in the jump row:
// the last one whose top has passed just under the sticky row.
const current = ref('')
let scroller: HTMLElement | null = null

function updateCurrent() {
  const top = (scroller?.getBoundingClientRect().top ?? 0) + 80
  let found = form.sections.value[0]?.id ?? ''
  for (const s of form.sections.value) {
    const el = document.getElementById(s.id)
    if (el && el.getBoundingClientRect().top <= top) found = s.id
  }
  current.value = found
}

function jump(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  router.replace({ hash: `#${id}` })
}

onMounted(async () => {
  await form.load()
  await nextTick()
  scroller = document.querySelector('main')
  scroller?.addEventListener('scroll', updateCurrent, { passive: true })
  const wanted = route.hash.slice(1)
  if (wanted) document.getElementById(wanted)?.scrollIntoView({ block: 'start' })
  updateCurrent()
})
onBeforeUnmount(() => scroller?.removeEventListener('scroll', updateCurrent))
</script>
