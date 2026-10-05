// Texts PlexHub shows about an item, shared by its card, row, dialog and
// the dashboard's "Continue watching" tile (src/components/plex/).
import { useI18n } from 'vue-i18n'
import type { PlexItem, PlexMedia } from '@/api/types'

export function usePlexText() {
  const { t, locale } = useI18n()

  function minutes(ms?: number): string {
    return ms ? t('plex.minutes', { n: Math.round(ms / 60000) }) : ''
  }

  // "S02E05" for an episode.
  function episodeCode(it: PlexItem): string {
    const pad = (n?: number) => String(n ?? 0).padStart(2, '0')
    return `S${pad(it.parentIndex)}E${pad(it.index)}`
  }

  // The secondary line: year · genres · length for a film, seasons and how
  // many episodes are seen for a series, "Dark · S01E02" for an episode.
  function details(it: PlexItem): string {
    switch (it.type) {
      case 'show':
        return [
          it.year ? String(it.year) : '',
          it.childCount ? t('plex.seasons', { n: it.childCount }, it.childCount) : '',
          it.leafCount ? t('plex.episodesSeen', { seen: it.viewedLeafCount ?? 0, total: it.leafCount }) : '',
        ]
          .filter(Boolean)
          .join(' · ')
      case 'episode':
        return [it.showTitle, episodeCode(it), minutes(it.duration)].filter(Boolean).join(' · ')
      default:
        return [it.year ? String(it.year) : '', it.genres?.slice(0, 2).join(', '), minutes(it.duration)].filter(Boolean).join(' · ')
    }
  }

  // Started but not finished: 0–1 of the way, else null.
  function progress(it: PlexItem): number | null {
    if (!it.viewOffset || !it.duration || it.watched) return null
    return Math.min(1, it.viewOffset / it.duration)
  }

  // "MKV · H.264 1080p · AC3 5.1" — what the box's player has to decode.
  function formats(m?: PlexMedia): string {
    if (!m) return ''
    const codec = (c?: string) => (c ? ({ h264: 'H.264', hevc: 'HEVC', mpeg2video: 'MPEG-2', mpeg4: 'MPEG-4' } as Record<string, string>)[c] ?? c.toUpperCase() : '')
    const res = m.videoResolution ? (/^\d+$/.test(m.videoResolution) ? `${m.videoResolution}p` : m.videoResolution.toUpperCase()) : ''
    const channels = m.audioChannels ? ({ 1: '1.0', 2: '2.0', 6: '5.1', 8: '7.1' } as Record<number, string>)[m.audioChannels] ?? `${m.audioChannels}ch` : ''
    return [m.container?.toUpperCase(), [codec(m.videoCodec), res].filter(Boolean).join(' '), [m.audioCodec?.toUpperCase(), channels].filter(Boolean).join(' ')]
      .filter(Boolean)
      .join(' · ')
  }

  function rating(r?: number): string {
    return r ? new Intl.NumberFormat(locale.value, { maximumFractionDigits: 1, minimumFractionDigits: 1 }).format(r) : ''
  }

  function title(it: PlexItem): string {
    return it.type === 'episode' ? `${it.showTitle} · ${episodeCode(it)} · ${it.title}` : it.title
  }

  return { minutes, episodeCode, details, progress, formats, rating, title }
}
