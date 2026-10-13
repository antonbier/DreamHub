<!--
  A picture fetched with the app's auth header (an <img src> can't send
  it): `load` returns the picture, shown once it arrives. Nothing while
  loading or when there is none.
-->
<template>
  <img v-if="src" :src="src" alt="" class="object-contain" :style="{ height: `${height}px`, maxWidth: `${height * 2.2}px` }" />
  <span v-else class="inline-block rounded bg-field" :style="{ height: `${height}px`, width: `${height * 1.6}px` }" />
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

const props = withDefaults(defineProps<{ load: () => Promise<Blob>; height?: number }>(), { height: 36 })
const src = ref<string | null>(null)

onMounted(async () => {
  try {
    src.value = URL.createObjectURL(await props.load())
  } catch {
    src.value = null
  }
})
onBeforeUnmount(() => {
  if (src.value) URL.revokeObjectURL(src.value)
})
</script>
