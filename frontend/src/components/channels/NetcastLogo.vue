<!--
  The logo of a Netcast channel: fetched by DreamHub's backend (the page never
  loads an outside address), only when the tile scrolls into view, kept for the
  session. Without a logo — or while it loads — the channel's initials.
-->
<template>
  <span ref="el" class="flex h-full w-full items-center justify-center">
    <img v-if="src" :src="src" alt="" class="max-h-full max-w-full object-contain" />
    <span v-else class="text-[20px] font-semibold text-faint" aria-hidden="true">{{ initials }}</span>
  </span>
</template>

<script lang="ts">
import { api } from '@/api/client'

const cache = new Map<string, Promise<string | null>>()
function logoURL(token: string, id: number) {
  const key = `${token}:${id}`
  let p = cache.get(key)
  if (!p) {
    p = api.netcast
      .logo(token, id)
      .then((b) => URL.createObjectURL(b))
      .catch(() => null) // none: remembered, not asked again
    cache.set(key, p)
  }
  return p
}
</script>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

const props = defineProps<{ token: string; id: number; name: string; hasLogo: boolean }>()
const el = ref<HTMLElement | null>(null)
const src = ref<string | null>(null)
let observer: IntersectionObserver | null = null

const initials = computed(() => {
  const words = props.name.replace(/[^\p{L}\p{N} ]/gu, ' ').split(/\s+/).filter(Boolean)
  const two = words.length > 1 ? words[0][0] + words[1][0] : (words[0] ?? '?').slice(0, 2)
  return two.toLocaleUpperCase()
})

async function load() {
  if (props.hasLogo) src.value = await logoURL(props.token, props.id)
}

onMounted(() => {
  if (!props.hasLogo) return
  if (typeof IntersectionObserver === 'undefined' || !el.value) {
    void load()
    return
  }
  observer = new IntersectionObserver((entries) => {
    if (entries.some((e) => e.isIntersecting)) {
      observer?.disconnect()
      void load()
    }
  })
  observer.observe(el.value)
})
onBeforeUnmount(() => observer?.disconnect())
</script>
