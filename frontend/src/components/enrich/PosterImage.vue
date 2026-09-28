<!--
  A poster or backdrop from enrichment, fetched through DreamHub's own
  image endpoint with the app's auth header (an <img src> can't carry it)
  and shown as an object URL. Object URLs are shared per image for the
  session, so the same poster in two places is fetched once. Without a
  path, or while loading, it's a plain `field`-colored box of the same
  size, so rows don't jump when the picture arrives.
-->
<template>
  <span class="block shrink-0 overflow-hidden rounded-sm bg-field" :style="{ width: `${width}px`, height: `${height}px` }">
    <img v-if="src" :src="src" :alt="alt" class="h-full w-full object-cover" loading="lazy" />
  </span>
</template>

<script lang="ts">
import { api } from '@/api/client'

// Session-wide cache: image key → object URL (or the pending fetch).
const urls = new Map<string, Promise<string | null>>()

function load(size: string, path: string): Promise<string | null> {
  const key = `${size}${path}`
  let p = urls.get(key)
  if (!p) {
    p = api.enrich
      .image(size, path)
      .then((blob) => URL.createObjectURL(blob))
      .catch(() => {
        urls.delete(key) // try again another time
        return null
      })
    urls.set(key, p)
  }
  return p
}
</script>

<script setup lang="ts">
import { ref, watch } from 'vue'

const props = withDefaults(
  defineProps<{ path?: string; size?: 'w92' | 'w185' | 'w342' | 'w780'; width?: number; height?: number; alt?: string }>(),
  { size: 'w92', width: 36, height: 52, alt: '' },
)

const src = ref<string | null>(null)
watch(
  () => [props.path, props.size],
  async () => {
    src.value = null
    if (!props.path) return
    const path = props.path
    const url = await load(props.size, path)
    if (props.path === path) src.value = url
  },
  { immediate: true },
)
</script>
