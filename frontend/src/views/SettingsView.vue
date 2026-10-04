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
import { useSettingsForm } from '@/composables/useSettingsForm'
import { useSettingsText } from '@/composables/useSettingsText'

const { t } = useI18n()
const text = useSettingsText()
const form = useSettingsForm()
const route = useRoute()
const router = useRouter()

// The section nearest the top of the scroll area, highlighted in the jump row.
const current = ref('')
let observer: IntersectionObserver | undefined

function jump(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  current.value = id
  router.replace({ hash: `#${id}` })
}

onMounted(async () => {
  await form.load()
  await nextTick()
  const visible = new Map<string, boolean>()
  observer = new IntersectionObserver(
    (entries) => {
      for (const e of entries) visible.set(e.target.id, e.isIntersecting)
      const first = form.sections.value.find((s) => visible.get(s.id))
      if (first) current.value = first.id
    },
    { rootMargin: '-60px 0px -60% 0px' },
  )
  for (const s of form.sections.value) {
    const el = document.getElementById(s.id)
    if (el) observer.observe(el)
  }
  const wanted = route.hash.slice(1)
  if (wanted) document.getElementById(wanted)?.scrollIntoView({ block: 'start' })
})
onBeforeUnmount(() => observer?.disconnect())
</script>
