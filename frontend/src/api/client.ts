// Thin fetch wrapper for /api/v1 — see backend/README.md, "API so far".
//
// - Every mutating request carries the CSRF header the backend requires
//   (X-DreamHub-Request); GET/HEAD don't need it.
// - Same-origin only: the frontend is served by the same Go binary it talks
//   to (go:embed), so no base URL or CORS handling is needed.
// - Errors follow the backend's envelope: {"error": {code, message,
//   requestId, details}}. ApiError carries the code (for the UI to
//   translate) and requestId (for support/logs), never the English message.
// - When Basic Auth credentials are on hand (authState.ts, set by
//   useAuth.ts's login()), every request carries them; a 401 from any
//   endpoint means they're gone or wrong, so they're dropped and the app
//   falls back to the login screen — see App.vue.
import { shallowRef } from 'vue'
import { authHeaders, handleUnauthorized } from './authState'
import type {
  AdminPackage,
  AdminScript,
  AdScanResult,
  BouquetEntry,
  ChannelBackup,
  DraftEntry,
  ChannelBouquet,
  ChannelInfo,
  ChannelIssue,
  Job,
  PiconSet,
  Profile,
  ProfileInput,
  ProfileList,
  ApiToken,
  AutoTimers,
  BatchRenameItem,
  CurrentService,
  SelfTestReport,
  ServiceUnit,
  ScriptRun,
  TerminalStatus,
  CoverItem,
  BackupFile,
  ConnectorState,
  MediaFolder,
  NameSuggestion,
  SuggestionSource,
  SimulatedTimer,
  TrashItem,
  Bouquet,
  ChangeEntry,
  ChannelSchedule,
  CrashLog,
  CreatedApiToken,
  DashboardLayout,
  DeviceInfo,
  Disk,
  DiscoveryStatus,
  EnrichExtras,
  EnrichInfo,
  EnrichQuery,
  HealthInfo,
  HubStatus,
  LogEntry,
  LogLevel,
  Navigation,
  Notification,
  Plan,
  NowNext,
  Recommendation,
  SavedPicks,
  SearchResult,
  SeriesDetail,
  TasteCounts,
  TasteProfile,
  SeriesMarker,
  SettingsView,
  Timer,
  UpcomingEpisode,
  ViewQuery,
  ViewResult,
  ViewSource,
  WhoAmI,
  PlexItem,
  PlexLibrary,
  PlexMeta,
  PlexPin,
  PlexServerChoice,
  PlexStatus,
} from './types'

export class ApiError extends Error {
  code: string
  status: number
  requestId?: string
  details?: Record<string, { code: string; params?: Record<string, unknown> }>

  constructor(
    status: number,
    code: string,
    message: string,
    requestId?: string,
    details?: Record<string, { code: string; params?: Record<string, unknown> }>,
  ) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.requestId = requestId
    this.details = details
  }
}

const CSRF_HEADER = 'X-DreamHub-Request'

// The most recent failed request, for the plain-language "couldn't load"
// hints (src/composables/useFriendlyError.ts): most views only keep a
// "failed" flag, this says why.
export const lastError = shallowRef<ApiError | null>(null)

function failed(err: ApiError): ApiError {
  lastError.value = err
  return err
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {}
  if (method !== 'GET' && method !== 'HEAD') {
    headers[CSRF_HEADER] = '1'
  }
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
  }
  Object.assign(headers, authHeaders())

  let res: Response
  try {
    res = await fetch(path, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    })
  } catch (err) {
    // Network failure (backend unreachable) — no HTTP response at all.
    throw failed(new ApiError(0, 'network_error', err instanceof Error ? err.message : String(err)))
  }

  if (res.status === 204) {
    return undefined as T
  }

  const requestId = res.headers.get('X-Request-ID') ?? undefined
  let json: unknown
  try {
    json = await res.json()
  } catch {
    json = undefined
  }

  if (res.status === 401) {
    // Wrong, revoked or (for a request sent before any login) simply
    // absent credentials — the login screen decides next; or no profile
    // chosen on this device — the profile picker does.
    handleUnauthorized((json as { error?: { code?: string } })?.error?.code)
  }

  if (!res.ok) {
    const envelope = (json as { error?: { code?: string; message?: string; details?: ApiError['details'] } })?.error
    throw failed(
      new ApiError(res.status, envelope?.code ?? 'unknown', envelope?.message ?? res.statusText, requestId, envelope?.details),
    )
  }
  return json as T
}

// For binary responses (ControlHub's screenshot). Fetched rather than
// set as a plain <img src> so it carries the same Authorization header as
// every other request — an <img> can't, and would just 401 with auth on.
async function requestBlob(path: string): Promise<Blob> {
  const headers = authHeaders()
  let res: Response
  try {
    res = await fetch(path, { headers, cache: 'no-store' })
  } catch (err) {
    throw failed(new ApiError(0, 'network_error', err instanceof Error ? err.message : String(err)))
  }
  if (!res.ok) {
    let envelope: { code?: string; message?: string } | undefined
    try {
      envelope = ((await res.json()) as { error?: typeof envelope })?.error
    } catch {
      envelope = undefined
    }
    if (res.status === 401) handleUnauthorized(envelope?.code)
    throw failed(new ApiError(res.status, envelope?.code ?? 'unknown', envelope?.message ?? res.statusText, res.headers.get('X-Request-ID') ?? undefined))
  }
  return res.blob()
}

export const api = {
  health: () => request<HealthInfo>('GET', '/api/v1/health'),
  whoami: () => request<WhoAmI>('GET', '/api/v1/whoami'),
  channels: {
    bouquets: () => request<{ bouquets: ChannelBouquet[]; editor: boolean }>('GET', '/api/v1/channels/bouquets'),
    bouquet: (ref: string) => request<{ entries: BouquetEntry[] }>('GET', `/api/v1/channels/bouquet?ref=${encodeURIComponent(ref)}`),
    all: (refresh = false) =>
      request<{ channels: ChannelInfo[]; refreshedAt: number; since: number }>('GET', `/api/v1/channels/all${refresh ? '?refresh=1' : ''}`),
    check: () => request<{ issues: ChannelIssue[] }>('GET', '/api/v1/channels/check'),
    picons: () => request<{ sets: PiconSet[]; total: number; chosen: string }>('GET', '/api/v1/channels/picons'),
    choosePicons: (set: string) => request<{ chosen: string }>('PUT', '/api/v1/channels/picons', { set }),
    picon: (ref: string, name: string) => requestBlob(`/api/v1/channels/picon?ref=${encodeURIComponent(ref)}&name=${encodeURIComponent(name)}`),
    // How the bouquet should look when done; the plan says what changes.
    planBouquet: (ref: string, kind: 'tv' | 'radio', entries: DraftEntry[]) =>
      request<{ plan: Plan }>('POST', '/api/v1/channels/bouquet/plan', { ref, kind, entries }),
    planBouquets: (action: 'create' | 'rename' | 'delete', kind: 'tv' | 'radio', ref: string, name: string) =>
      request<{ plan: Plan }>('POST', '/api/v1/channels/bouquets/plan', { action, kind, ref, name }),
    backups: () => request<{ backups: ChannelBackup[] }>('GET', '/api/v1/channels/backups'),
    planRestore: (filename: string) => request<{ plan: Plan }>('POST', '/api/v1/channels/backups/restore/plan', { filename }),
  },
  profiles: {
    list: () => request<ProfileList>('GET', '/api/v1/profiles'),
    startSession: (id: number, pin?: string) =>
      request<{ token: string; profile: Profile }>('POST', `/api/v1/profiles/${id}/session`, pin ? { pin } : undefined),
    endSession: () => request<void>('DELETE', '/api/v1/profiles/session'),
    create: (input: ProfileInput) => request<Profile>('POST', '/api/v1/profiles', input),
    update: (id: number, input: ProfileInput) => request<Profile>('PATCH', `/api/v1/profiles/${id}`, input),
    remove: (id: number) => request<void>('DELETE', `/api/v1/profiles/${id}`),
    setMyPin: (pin: string) => request<Profile>('PUT', '/api/v1/profiles/me/pin', { pin }),
  },
  deviceInfo: () => request<DeviceInfo>('GET', '/api/v1/deviceinfo'),
  navigation: () => request<Navigation>('GET', '/api/v1/navigation'),
  settings: {
    get: () => request<SettingsView>('GET', '/api/v1/settings'),
    // A null value resets that key to its default (backend/README.md).
    update: (values: Record<string, unknown>) =>
      request<SettingsView>('PATCH', '/api/v1/settings', { values }),
  },
  hubs: () => request<{ hubs: HubStatus[] }>('GET', '/api/v1/hubs'),
  connectors: () => request<{ connectors: ConnectorState[] }>('GET', '/api/v1/connectors'),
  timers: () => request<{ timers: Timer[]; tuners: number }>('GET', '/api/v1/timers'),
  // backend/internal/hubs.createRequest (internal/hubs/timer.go) — used
  // directly by TimerHub's own future UI and by DiscoveryHub's "create
  // timer from a recommendation" hand-off (CONCEPT.md: DiscoveryHub calls
  // TimerHub's API, not its Go code).
  timerPlan: (req: {
    serviceReference: string
    serviceName: string
    name: string
    description?: string
    begin: number
    end: number
    marginBefore?: number
    marginAfter?: number
    // The EPG event being recorded, when known (GuideHub) — the box links
    // the timer to it.
    eventId?: number
  }) => request<{ plan: Plan }>('POST', '/api/v1/timers/plan', req),
  // Identify the timer exactly as GET /api/v1/timers lists it.
  // Disable (enabled=false) or enable a timer: preview, then apply.
  timerTogglePlan: (t: { serviceReference: string; begin: number; end: number }, enabled: boolean) =>
    request<{ plan: Plan }>('POST', '/api/v1/timers/toggle/plan', { serviceReference: t.serviceReference, begin: t.begin, end: t.end, enabled }),
  timerDeletePlan: (t: { serviceReference: string; begin: number; end: number }) =>
    request<{ plan: Plan }>('POST', '/api/v1/timers/delete/plan', {
      serviceReference: t.serviceReference,
      begin: t.begin,
      end: t.end,
    }),
  autoTimers: () => request<AutoTimers>('GET', '/api/v1/timers/autotimers'),
  autoTimerSimulate: () => request<{ timers: SimulatedTimer[] }>('GET', '/api/v1/timers/autotimers/simulate'),
  plans: {
    // backend/internal/plan.Result.
    apply: (id: string) => request<{ planId: string; action: string }>('POST', `/api/v1/plans/${encodeURIComponent(id)}/apply`),
  },
  media: {
    // One folder; no dir means the box's first recording folder.
    // A recording's own cover art from the box's disk (not TMDB).
    cover: (path: string, kind: 'poster' | 'backdrop' = 'poster') =>
      requestBlob(`/api/v1/media/cover?path=${encodeURIComponent(path)}&kind=${kind}`),
    folder: (dir?: string) =>
      request<MediaFolder>('GET', `/api/v1/media/movies${dir ? `?dir=${encodeURIComponent(dir)}` : ''}`),
    suggestName: (path: string, source: SuggestionSource) =>
      request<{ suggestions: NameSuggestion[] }>('POST', '/api/v1/media/rename/suggest', { path, source }),
    // Many recordings at once, named by the naming scheme (Settings → Recordings).
    batchRenamePlan: (paths: string[], source: 'metadata' | 'tmdb', folders: boolean) =>
      request<{ plan?: Plan; items: BatchRenameItem[]; folders?: string[] }>('POST', '/api/v1/media/rename/batch/plan', {
        paths,
        source,
        folders,
      }),
    // Covers from enrichment written next to recordings that have none.
    saveCoversPlan: (paths: string[]) => request<{ plan?: Plan; items: CoverItem[] }>('POST', '/api/v1/media/covers/plan', { paths }),
    // title also changes the .ts.meta title the box's own list shows.
    renamePlan: (path: string, newName: string, title?: string) =>
      request<{ plan: Plan }>('POST', '/api/v1/media/rename/plan', { path, newName, title }),
    trashPlan: (paths: string[]) => request<{ plan: Plan }>('POST', '/api/v1/media/trash/plan', { paths }),
    trash: () => request<{ items: TrashItem[] }>('GET', '/api/v1/media/trash'),
    restorePlan: (paths: string[]) => request<{ plan: Plan }>('POST', '/api/v1/media/trash/restore/plan', { paths }),
    emptyPlan: (paths: string[]) => request<{ plan: Plan; bytes: number }>('POST', '/api/v1/media/trash/empty/plan', { paths }),
    disk: () => request<{ disks: Disk[] }>('GET', '/api/v1/media/disk'),
    // Ad detection: queue an analysis per recording (it changes nothing),
    // and read one recording's last result.
    adScanStart: (paths: string[]) =>
      request<{ items: { path: string; jobId?: number; existing?: boolean; reason?: string }[] }>('POST', '/api/v1/media/adscan', { paths }),
    // Cutting ticked breaks out of a recording: a preview; applying it
    // queues the cut (a "media.trim" job).
    trimPlan: (path: string, blocks: { start: number; end: number }[]) =>
      request<{ plan: Plan; keep: { start: number; end: number }[] }>('POST', '/api/v1/media/trim/plan', { path, blocks }),
    // Marking ticked breaks in the recording's own cut list, without
    // changing the recording itself — for jumping past them with the
    // box's own remote. Runs at once on apply, no background job.
    markPlan: (path: string, blocks: { start: number; end: number }[]) =>
      request<{ plan: Plan }>('POST', '/api/v1/media/adscan/mark/plan', { path, blocks }),
    adScanResult: (path: string) =>
      request<{ path: string; analyzedAt: string; result: AdScanResult }>('GET', `/api/v1/media/adscan?path=${encodeURIComponent(path)}`),
  },
  // The background queue; changes arrive live on the event stream.
  jobs: {
    list: () => request<{ jobs: Job[]; blockedBy: string }>('GET', '/api/v1/jobs'),
    cancel: (id: number) => request<Job>('POST', `/api/v1/jobs/${id}/cancel`),
  },
  guide: {
    bouquets: () => request<{ bouquets: Bouquet[] }>('GET', '/api/v1/guide/bouquets'),
    // service narrows the answer to one channel (the channel view).
    grid: (bouquet: string, start: number, end: number, service?: string) =>
      request<{ channels: ChannelSchedule[] }>(
        'GET',
        `/api/v1/guide/grid?bouquet=${encodeURIComponent(bouquet)}&start=${start}&end=${end}` +
          (service ? `&service=${encodeURIComponent(service)}` : ''),
      ),
    // Now and next on every channel of a bouquet, one call to the box.
    nowNext: (bouquet: string) => request<{ channels: NowNext[] }>('GET', `/api/v1/guide/nownext?bouquet=${encodeURIComponent(bouquet)}`),
    // The channel's logo from the box (404 when it has none).
    picon: (ref: string, name: string) => requestBlob(`/api/v1/guide/picon?ref=${encodeURIComponent(ref)}&name=${encodeURIComponent(name)}`),
  },
  radio: {
    // The radio bouquets, then all radio stations (`all`, unnamed).
    bouquets: () => request<{ bouquets: Bouquet[]; all: string }>('GET', '/api/v1/radio/bouquets'),
    // Every station of a radio bouquet with now/next (null without EPG),
    // and what the box is playing.
    stations: (bouquet: string) =>
      request<{ stations: NowNext[]; playing: string }>('GET', `/api/v1/radio/stations?bouquet=${encodeURIComponent(bouquet)}`),
    // Radio recordings that haven't ended yet, soonest first.
    timers: () => request<{ timers: Timer[] }>('GET', '/api/v1/radio/timers'),
    picon: (ref: string, name: string) => requestBlob(`/api/v1/radio/picon?ref=${encodeURIComponent(ref)}&name=${encodeURIComponent(name)}`),
  },
  stream: {
    // The TV bouquets (no headings).
    bouquets: () => request<{ bouquets: Bouquet[] }>('GET', '/api/v1/stream/bouquets'),
    // A bouquet's channels with now/next, and what the box shows.
    channels: (bouquet: string) =>
      request<{ channels: NowNext[]; playing: string }>('GET', `/api/v1/stream/channels?bouquet=${encodeURIComponent(bouquet)}`),
    picon: (ref: string, name: string) => requestBlob(`/api/v1/stream/picon?ref=${encodeURIComponent(ref)}&name=${encodeURIComponent(name)}`),
    // The streams themselves are read by src/streaming/tsPlayer.ts.
  },
  series: {
    markers: () => request<{ markers: SeriesMarker[] }>('GET', '/api/v1/series/markers'),
    detail: (id: number) => request<SeriesDetail>('GET', `/api/v1/series/markers/${id}`),
    // The series' own cover art from its folder on the box (folder.jpg).
    cover: (id: number) => requestBlob(`/api/v1/series/markers/${id}/cover`),
    upcoming: () => request<{ episodes: UpcomingEpisode[] }>('GET', '/api/v1/series/upcoming'),
  },
  discovery: {
    status: () => request<DiscoveryStatus>('GET', '/api/v1/discovery/status'),
    taste: () => request<TasteProfile>('GET', '/api/v1/discovery/taste'),
    // The last generated picks, without a new AI call.
    saved: () => request<{ saved: SavedPicks | null }>('GET', '/api/v1/discovery/recommendations'),
    generate: (bouquet: string, useTaste: boolean) =>
      request<DiscoveryStatus & { recommendations: Recommendation[]; taste: TasteCounts; bouquet: string; generatedAt: number }>(
        'POST',
        '/api/v1/discovery/recommendations',
        { bouquet, useTaste },
      ),
  },
  push: {
    // A test notification with the saved settings.
    test: () => request<{ sent: boolean }>('POST', '/api/v1/push/test'),
  },
  enrich: {
    // Read-only despite the POST (a batch of titles).
    lookup: (items: EnrichQuery[]) =>
      request<{ enabled: boolean; results: (EnrichInfo | null)[] }>('POST', '/api/v1/enrich/lookup', { items }),
    // Served by DreamHub from its own cache; the browser never contacts TMDB.
    image: (size: string, path: string) => requestBlob(`/api/v1/enrich/image/${size}${path}`),
    // Ratings (OMDb) and a logo (fanart.tv) for one title's detail view.
    extras: (kind: 'movie' | 'tv', id: number) =>
      request<{ extras: EnrichExtras | null }>('GET', `/api/v1/enrich/extras?kind=${kind}&id=${id}`),
    logo: (path: string) => requestBlob(`/api/v1/enrich/logo?path=${encodeURIComponent(path)}`),
  },
  dashboard: {
    get: () => request<DashboardLayout>('GET', '/api/v1/dashboard'),
    save: (layout: DashboardLayout) => request<DashboardLayout>('PUT', '/api/v1/dashboard', layout),
    reset: () => request<DashboardLayout>('DELETE', '/api/v1/dashboard'),
    // The viewers' start page, designed by an admin.
    template: () => request<DashboardLayout>('GET', '/api/v1/dashboard/template'),
    saveTemplate: (layout: DashboardLayout) => request<DashboardLayout>('PUT', '/api/v1/dashboard/template', layout),
    sources: () => request<{ sources: ViewSource[] }>('GET', '/api/v1/dashboard/sources'),
    // Read-only despite the POST (structured criteria, not a query string).
    view: (source: string, query: ViewQuery, timeZone: string) =>
      request<ViewResult>('POST', '/api/v1/dashboard/view', { source, query, timeZone }),
  },
  search: (query: string) =>
    request<{ results: SearchResult[] }>('GET', `/api/v1/search?q=${encodeURIComponent(query)}`),
  tokens: {
    list: () => request<{ tokens: ApiToken[] }>('GET', '/api/v1/tokens'),
    create: (name: string) => request<CreatedApiToken>('POST', '/api/v1/tokens', { name }),
    revoke: (id: number) => request<void>('DELETE', `/api/v1/tokens/${id}`),
  },
  // user: a profile's id, 0 for the system's own entries, -1 everyone's.
  changes: (limit = 20, user = -1) =>
    request<{ changes: ChangeEntry[] }>('GET', `/api/v1/changes?limit=${limit}${user >= 0 ? `&user=${user}` : ''}`),
  system: {
    backup: () => request<BackupFile>('GET', '/api/v1/system/backup'),
    restorePlan: (file: BackupFile) => request<{ plan: Plan }>('POST', '/api/v1/system/backup/restore', file),
    // The box self-test (admin only): the last report, a new run, and the
    // run's captured answers as a zip.
    selfTest: () => request<{ report: SelfTestReport | null }>('GET', '/api/v1/system/selftest'),
    runSelfTest: () => request<{ report: SelfTestReport }>('POST', '/api/v1/system/selftest'),
    selfTestZip: () => requestBlob('/api/v1/system/selftest/download'),
  },
  admin: {
    // The box's services (enigma2, DreamHub, Samba, …) and their state.
    services: () => request<{ services: ServiceUnit[] }>('GET', '/api/v1/admin/services'),
    // Preview restarting (or starting) one; applied through plans.apply.
    servicePlan: (name: string) => request<{ plan: Plan }>('POST', '/api/v1/admin/services/plan', { name }),
    logLevel: {
      get: () => request<LogLevel>('GET', '/api/v1/admin/log-level'),
      set: (level: 'info' | 'debug', minutes?: number) => request<LogLevel>('PUT', '/api/v1/admin/log-level', { level, minutes }),
    },
    logs: (filter: { unit?: string; level?: string; text?: string; limit?: number }) => {
      const params = new URLSearchParams()
      if (filter.unit) params.set('unit', filter.unit)
      if (filter.level) params.set('level', filter.level)
      if (filter.text) params.set('text', filter.text)
      params.set('limit', String(filter.limit ?? 200))
      return request<{ entries: LogEntry[] }>('GET', `/api/v1/admin/logs?${params}`)
    },
    // Live tail's stream path (useEventStream.ts reads it directly, not through request()).
    logsStreamPath: (filter: { unit?: string; level?: string; text?: string }) => {
      const params = new URLSearchParams()
      if (filter.unit) params.set('unit', filter.unit)
      if (filter.level) params.set('level', filter.level)
      if (filter.text) params.set('text', filter.text)
      return `/api/v1/admin/logs/stream?${params}`
    },
    crashLogs: () => request<{ logs: CrashLog[] }>('GET', '/api/v1/admin/crashlogs'),
    crashLog: (name: string) => request<{ name: string; content: string }>('GET', `/api/v1/admin/crashlogs/${encodeURIComponent(name)}`),
    // The script library (docs/ADMINHUB_PLAN.md, A3): upload, list, delete;
    // running one goes through plans.apply like everything else that changes
    // something.
    scripts: () => request<{ scripts: AdminScript[] }>('GET', '/api/v1/admin/scripts'),
    uploadScript: (name: string, content: string) => request<AdminScript>('POST', '/api/v1/admin/scripts', { name, content }),
    deleteScript: (id: number) => request<void>('DELETE', `/api/v1/admin/scripts/${id}`),
    runScriptPlan: (id: number) => request<{ plan: Plan }>('POST', `/api/v1/admin/scripts/${id}/run/plan`),
    scriptRuns: (id: number) => request<{ runs: ScriptRun[] }>('GET', `/api/v1/admin/scripts/${id}/runs`),
    // The terminal (docs/ADMINHUB_PLAN.md, A4, #36): off by default, needs
    // auth turned on and the password typed again to enable — the one
    // explicit exception to preview-and-confirm (PRINCIPLES.md), since
    // there's nothing to preview about opening a shell.
    terminalStatus: () => request<TerminalStatus>('GET', '/api/v1/admin/terminal/status'),
    enableTerminal: (password: string) => request<{ enabled: boolean }>('POST', '/api/v1/admin/terminal/enable', { password }),
    disableTerminal: () => request<{ enabled: boolean }>('POST', '/api/v1/admin/terminal/disable'),
    // Not read through request(): the iframe/browser hits this directly,
    // proxied straight to shellinaboxd.
    terminalProxyPath: '/api/v1/admin/terminal/proxy/',
    // The package browser (docs/ADMINHUB_PLAN.md, A2): read-only so far —
    // install/remove as a plan is a later slice.
    packages: (filter: { q?: string; category?: string; status?: string } = {}) => {
      const params = new URLSearchParams()
      if (filter.q) params.set('q', filter.q)
      if (filter.category) params.set('category', filter.category)
      if (filter.status) params.set('status', filter.status)
      return request<{ packages: AdminPackage[]; categories: string[] }>('GET', `/api/v1/admin/packages?${params}`)
    },
  },
  notifications: {
    list: (limit = 20) => request<{ notifications: Notification[]; unread: number }>('GET', `/api/v1/notifications?limit=${limit}`),
    markRead: () => request<{ unread: number }>('POST', '/api/v1/notifications/read'),
  },
  plex: {
    status: () => request<PlexStatus>('GET', '/api/v1/plex/status'),
    sync: () => request<PlexStatus>('POST', '/api/v1/plex/sync'),
    libraries: () => request<{ libraries: PlexLibrary[]; meta: PlexMeta }>('GET', '/api/v1/plex/libraries'),
    selectLibraries: (selected: string[]) =>
      request<{ libraries: PlexLibrary[]; meta: PlexMeta }>('PUT', '/api/v1/plex/libraries', { selected }),
    items: (library: string) =>
      request<{ items: PlexItem[]; meta: PlexMeta }>('GET', `/api/v1/plex/libraries/${encodeURIComponent(library)}/items`),
    item: (key: string) =>
      request<{ item: PlexItem; children?: PlexItem[]; meta: PlexMeta }>('GET', `/api/v1/plex/items/${encodeURIComponent(key)}`),
    onDeck: () => request<{ items: PlexItem[]; meta: PlexMeta }>('GET', '/api/v1/plex/ondeck'),
    image: (path: string, width: number) =>
      requestBlob(`/api/v1/plex/image?path=${encodeURIComponent(path)}&w=${width}`),
    // One film or episode, directly (undo = the inverse call).
    setWatched: (key: string, watched: boolean) =>
      request<{ item: PlexItem }>('POST', `/api/v1/plex/items/${encodeURIComponent(key)}/watched`, { watched }),
    // A series or season: a preview, applied through plans.apply.
    watchedPlan: (key: string, watched: boolean) =>
      request<{ plan: Plan }>('POST', `/api/v1/plex/items/${encodeURIComponent(key)}/watched/plan`, { watched }),
    // media/part pick a version/part other than the first (#31: a film in
    // several versions or parts); omitted, the backend plays the first of
    // each, as before.
    play: (key: string, choice?: { media: number; part: number }) =>
      request<{ playing: string }>('POST', `/api/v1/plex/items/${encodeURIComponent(key)}/play`, choice),
    login: {
      start: () => request<PlexPin>('POST', '/api/v1/plex/login'),
      check: (id: number) =>
        request<{ state: 'waiting' | 'linked' | 'expired'; servers?: PlexServerChoice[] }>('GET', `/api/v1/plex/login/${id}`),
      use: (id: number, serverId: string, url: string) =>
        request<{ connected: string; url: string }>('POST', `/api/v1/plex/login/${id}/use`, { serverId, url }),
    },
  },
  control: {
    screenshot: () => requestBlob('/api/v1/control/screenshot'),
    power: () => request<{ inStandby: boolean }>('GET', '/api/v1/control/power'),
    // What the box shows right now, with now/next (the "Now on the box" tile).
    current: () => request<CurrentService>('GET', '/api/v1/control/current'),
    sendKey: (code: number) => request<{ sent: number }>('POST', '/api/v1/control/key', { code }),
    zap: (serviceReference: string) => request<{ zapped: string }>('POST', '/api/v1/control/zap', { serviceReference }),
    message: (text: string, kind: 'info' | 'warning' | 'error', timeout: number) =>
      request<{ sent: boolean }>('POST', '/api/v1/control/message', { text, kind, timeout }),
  },
}
