<!--
  One AI recommendation as a card (DiscoveryHub): the shared ProgrammeCard
  with the AI's reason as its text.
-->
<template>
  <ProgrammeCard
    :title="rec.title"
    :service-name="rec.serviceName"
    :start="rec.start"
    :duration="rec.duration"
    :time="time"
    :now="now"
    :info="info"
    :has-timer="hasTimer"
    @open="$emit('open')"
  >
    {{ rec.reason }}
  </ProgrammeCard>
</template>

<script setup lang="ts">
import ProgrammeCard from '@/components/programme/ProgrammeCard.vue'
import type { EnrichInfo, Recommendation } from '@/api/types'

defineProps<{ rec: Recommendation; info?: EnrichInfo | null; time: string; now: number; hasTimer: boolean }>()
defineEmits<{ open: [] }>()
</script>
