<!--
  A rating from enrichment: "★ 6.9" in text color with the vote count in
  `faint`, formatted through Intl. Nothing for no rating or too few votes
  to mean anything.
-->
<template>
  <span v-if="shown" class="inline-flex items-center gap-1 whitespace-nowrap tabular-nums" :title="t('enrich.ratingTitle', { rating: value, votes: voteText })">
    <span class="text-warn" aria-hidden="true">★</span>
    <span class="text-text">{{ value }}</span>
    <span v-if="votes" class="text-faint">({{ voteText }})</span>
    <span class="sr-only">{{ t('enrich.ratingTitle', { rating: value, votes: voteText }) }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ rating?: number; votes?: number }>()
const { t, locale } = useI18n()

// A 10/10 from three votes says nothing.
const MIN_VOTES = 5
const shown = computed(() => !!props.rating && (props.votes ?? 0) >= MIN_VOTES)
const value = computed(() => new Intl.NumberFormat(locale.value, { maximumFractionDigits: 1, minimumFractionDigits: 1 }).format(props.rating ?? 0))
const voteText = computed(() => new Intl.NumberFormat(locale.value, { notation: 'compact' }).format(props.votes ?? 0))
</script>
