// The credit line for enrichment: where the details came from. With
// DreamMediaInfo, the text is the plugin's own; pictures still come from TMDB.
import type { EnrichInfo } from '@/api/types'

export function sourceName(info: EnrichInfo): string {
  return info.source === 'dreammediainfo' ? (info.poster ? 'DreamMediaInfo, TMDB' : 'DreamMediaInfo') : 'TMDB'
}
