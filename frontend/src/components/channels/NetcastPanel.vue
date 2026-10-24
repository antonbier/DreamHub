<!--
  Netcast's sources on the box, read only (docs/NETCAST_PLAN.md, N3): the
  playlists, APIs and guide sources the Gemini Netcast plugin has set up, from
  its own files. Nothing is changed here — writing needs the box's write route
  (box settings S3) — and a login is never shown, only that a source has one.
-->
<template>
  <section class="mt-4">
    <p class="text-[13.5px] text-muted">{{ t('channels.netcast.explain') }}</p>
    <p v-if="loading" class="mt-3 text-[13.5px] text-muted">{{ t('common.loading') }}</p>
    <p v-else-if="failed" class="mt-3 text-[13.5px] text-muted"><ErrorText /></p>
    <p v-else-if="data && !data.available" class="mt-3 rounded-sm border border-line bg-tile px-4 py-3 text-[13.5px] text-muted">{{ t('channels.netcast.none') }}</p>
    <template v-else-if="data">
      <p v-if="!data.listed" class="mt-3 text-[12.5px] text-faint">{{ t('channels.netcast.notListed') }}</p>
      <section v-for="f in data.files" :key="f.name" class="mt-4">
        <h2 class="text-[15px] font-semibold">
          {{ t(`channels.netcast.kind.${f.kind}`) }}
          <span class="ms-2 font-mono text-[12px] font-normal text-faint">{{ f.name }}</span>
        </h2>
        <p v-if="f.sources.length === 0" class="text-[13px] text-muted">{{ t('channels.netcast.empty') }}</p>
        <ul v-else class="mt-1 divide-y divide-line-soft rounded-sm border border-line bg-tile">
          <li v-for="(s, i) in f.sources" :key="s.id ?? i" class="px-3 py-2">
            <div class="flex flex-wrap items-baseline gap-x-3 gap-y-1">
              <span class="text-[14.5px] font-semibold text-text">{{ s.name }}</span>
              <span v-if="s.mediaType" class="rounded-sm border border-line px-1 text-[11.5px] text-muted">{{ s.mediaType }}</span>
              <span v-if="s.hasLogin" class="rounded-sm border border-line px-1 text-[11.5px] text-muted">{{ t('channels.iptv.login') }}</span>
            </div>
            <p v-if="s.address" class="truncate text-[12.5px] text-faint">{{ s.address }}</p>
            <p v-if="s.desc" class="text-[12.5px] text-muted">{{ s.desc }}</p>
          </li>
        </ul>
        <dl v-if="f.settings" class="mt-1 flex flex-wrap gap-x-4 gap-y-0.5 text-[12.5px] text-muted">
          <div v-for="(v, k) in f.settings" :key="k" class="flex gap-1">
            <dt class="font-mono text-faint">{{ k }}</dt>
            <dd>{{ v }}</dd>
          </div>
        </dl>
      </section>
      <p v-if="data.unreadable.length > 0" class="mt-4 text-[12.5px] text-warn">{{ t('channels.netcast.unreadable', { names: data.unreadable.join(', ') }) }}</p>
    </template>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import type { NetcastSources } from '@/api/types'
import ErrorText from '@/components/ErrorText.vue'

const { t } = useI18n()
const data = ref<NetcastSources | null>(null)
const loading = ref(true)
const failed = ref(false)

onMounted(async () => {
  try {
    data.value = await api.channels.netcastSources()
  } catch {
    failed.value = true
  } finally {
    loading.value = false
  }
})
</script>
