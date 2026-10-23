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
import { withBox } from '@/lib/box'
import type {
  TunerState,
  AdminFeed,
  AdminPackage,
  PackageCategory,
  DockerSetupState,
  DockerSetupStep,
  PackageRun,
  PackageStep,
  AdminScript,
  ExtensionCard,
  ExtensionSetting,
  LogoResult,
  AppStorageChoice,
  CatalogApp,
  InstallRequest,
  ParsedApp,
  AppSearchResult,
  AppManifest,
  AppConsoleStatus,
  InstalledApp,
  AppEditView,
  UnusedImages,
  UpdatePlan,
  UpdatesView,
  AdScanResult,
  BouquetEntry,
  ChannelBackup,
  ChannelSyncResult,
  DraftEntry,
  ChannelBouquet,
  ChannelInfo,
  ChannelIssue,
  Job,
  PiconSet,
  BoxInfo,
  BoxWakeResult,
  BoxInput,
  BoxStatus,
  PairingCode,
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
  BackupSectionsList,
  StoredBackup,
  RestorableApp,
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
  EPGEvent,
  TunerSignal,
  BoxSetting,
  BoxSettingHit,
  BoxSettingSection,
  BoxSettingsOverview,
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
  const raw = body instanceof Blob // an uploaded picture: sent as it is
  if (body !== undefined) {
    headers['Content-Type'] = raw ? body.type || 'application/octet-stream' : 'application/json'
  }
  Object.assign(headers, authHeaders())

  let res: Response
  try {
    res = await fetch(withBox(path), {
      method,
      headers,
      body: body === undefined ? undefined : raw ? body : JSON.stringify(body),
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
    res = await fetch(withBox(path), { headers, cache: 'no-store' })
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

// The reference to put in a display URL (a channel's logo). A plain DVB
// service reference is sent as it is. An IPTV one carries the stream address
// after its ten fields — possibly with rtsp://user:pass@host — and that must
// not end up in a URL (address bar, proxies, logs): only the ten fields go,
// the channel's name (sent alongside) tells channels apart. Choosing or
// removing a logo still sends the full reference; the box hashes the tail.
export function displayRef(ref: string): string {
  const parts = ref.split(':')
  if (parts.length <= 10) return ref
  const tail = parts.slice(10).join(':')
  if (!tail.includes('//') && !/%3a/i.test(tail)) return ref
  return parts.slice(0, 10).join(':') + ':'
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
    picon: (ref: string, name: string) => requestBlob(`/api/v1/channels/picon?ref=${encodeURIComponent(displayRef(ref))}&name=${encodeURIComponent(name)}`),
    // A logo chosen by hand for one channel (DreamHub's own choice; the TV is untouched).
    logoSets: (ref: string, name: string) =>
      request<{ sets: string[]; custom: boolean }>('GET', `/api/v1/channels/logo/sets?ref=${encodeURIComponent(ref)}&name=${encodeURIComponent(name)}`),
    piconInSet: (ref: string, name: string, set: string) =>
      requestBlob(`/api/v1/channels/picon?ref=${encodeURIComponent(ref)}&name=${encodeURIComponent(name)}&set=${encodeURIComponent(set)}`),
    customLogos: () => request<{ keys: string[] }>('GET', '/api/v1/channels/logo/custom'),
    chooseLogo: (ref: string, name: string, set: string) => request<{ saved: boolean }>('PUT', '/api/v1/channels/logo', { ref, name, set }),
    uploadLogo: (ref: string, file: Blob) => request<{ saved: boolean }>('POST', `/api/v1/channels/logo/upload?ref=${encodeURIComponent(ref)}`, file),
    searchLogos: (name: string) => request<{ results: LogoResult[] }>('GET', `/api/v1/channels/logo/search?name=${encodeURIComponent(name)}`),
    logoPreview: (path: string) => requestBlob(`/api/v1/channels/logo/preview?path=${encodeURIComponent(path)}`),
    fetchLogo: (ref: string, path: string) => request<{ saved: boolean }>('POST', '/api/v1/channels/logo/online', { ref, path }),
    resetLogo: (ref: string) => request<void>('DELETE', `/api/v1/channels/logo?ref=${encodeURIComponent(ref)}`),
    // How the bouquet should look when done; the plan says what changes.
    planBouquet: (ref: string, kind: 'tv' | 'radio', entries: DraftEntry[]) =>
      request<{ plan: Plan }>('POST', '/api/v1/channels/bouquet/plan', { ref, kind, entries }),
    planBouquets: (action: 'create' | 'rename' | 'delete', kind: 'tv' | 'radio', ref: string, name: string) =>
      request<{ plan: Plan }>('POST', '/api/v1/channels/bouquets/plan', { action, kind, ref, name }),
    planSync: (from: string, bouquets: string[] = []) =>
      request<ChannelSyncResult>('POST', '/api/v1/channels/sync/plan', { from, bouquets }),
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
  boxes: {
    list: () => request<{ boxes: BoxInfo[]; default: string }>('GET', '/api/v1/boxes'),
    overview: () => request<{ boxes: BoxStatus[] }>('GET', '/api/v1/boxes/overview'),
    test: (input: BoxInput) => request<{ deviceName: string; tuners: number }>('POST', '/api/v1/boxes/test', input),
    add: (input: BoxInput) => request<BoxInfo>('POST', '/api/v1/boxes', input),
    update: (id: string, input: BoxInput) => request<BoxInfo>('PATCH', `/api/v1/boxes/${encodeURIComponent(id)}`, input),
    planRemove: (id: string) => request<{ plan: Plan }>('POST', `/api/v1/boxes/${encodeURIComponent(id)}/remove/plan`),
    pair: (id: string, input: { code: string; url?: string }) =>
      request<BoxInfo>('POST', `/api/v1/boxes/${encodeURIComponent(id)}/pair`, input),
    unpair: (id: string) => request<BoxInfo>('DELETE', `/api/v1/boxes/${encodeURIComponent(id)}/pair`),
    wake: (id: string) => request<BoxWakeResult>('POST', `/api/v1/boxes/${encodeURIComponent(id)}/wake`),
  },
  // This box shows a code that the DreamHub of another box pairs with.
  peer: {
    code: () => request<PairingCode>('POST', '/api/v1/peer/code'),
  },
  deviceInfo: () => request<DeviceInfo>('GET', '/api/v1/deviceinfo'),
  // Reception of the tuner showing the current channel; signal null without numbers.
  tunerSignal: () => request<{ available: boolean; signal: TunerSignal | null }>('GET', '/api/v1/deviceinfo/signal'),
  // The receiver's own setup menus, read only (docs/BOXSETTINGS_PLAN.md, S1).
  boxSettings: {
    overview: (refresh = false) => request<BoxSettingsOverview>('GET', `/api/v1/boxsettings${refresh ? '?refresh=1' : ''}`),
    section: (key: string) =>
      request<{ section: BoxSettingSection; items: BoxSetting[] }>('GET', `/api/v1/boxsettings/sections/${encodeURIComponent(key)}`),
    search: (q: string) => request<{ available: boolean; hits: BoxSettingHit[] }>('GET', `/api/v1/boxsettings/search?q=${encodeURIComponent(q)}`),
  },
  navigation: () => request<Navigation>('GET', '/api/v1/navigation'),
  settings: {
    get: () => request<SettingsView>('GET', '/api/v1/settings'),
    // A null value resets that key to its default (backend/README.md).
    update: (values: Record<string, unknown>) =>
      request<SettingsView>('PATCH', '/api/v1/settings', { values }),
  },
  hubs: () => request<{ hubs: HubStatus[] }>('GET', '/api/v1/hubs'),
  connectors: () => request<{ connectors: ConnectorState[] }>('GET', '/api/v1/connectors'),
  timers: () => request<{ timers: Timer[]; tuners: number; tunerStates?: TunerState[] }>('GET', '/api/v1/timers'),
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
  // Every finished one-off timer, as one plan; no plan when there is none.
  timerCleanupPlan: () => request<{ plan?: Plan; count: number }>('POST', '/api/v1/timers/cleanup/plan', {}),
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
    // Other airings of a programme, not over yet; available: false when the box has no such search.
    similar: (ref: string, event: number) =>
      request<{ available: boolean; events: EPGEvent[] }>('GET', `/api/v1/guide/similar?ref=${encodeURIComponent(ref)}&event=${event}`),
    // The channel's logo from the box (404 when it has none).
    picon: (ref: string, name: string) => requestBlob(`/api/v1/guide/picon?ref=${encodeURIComponent(displayRef(ref))}&name=${encodeURIComponent(name)}`),
  },
  radio: {
    // The radio bouquets, then all radio stations (`all`, unnamed).
    bouquets: () => request<{ bouquets: Bouquet[]; all: string }>('GET', '/api/v1/radio/bouquets'),
    // Every station of a radio bouquet with now/next (null without EPG),
    // and what the box is playing.
    stations: (bouquet: string) =>
      request<{ stations: NowNext[]; playing: string }>('GET', `/api/v1/radio/stations?bouquet=${encodeURIComponent(bouquet)}`),
    // The station's radio text (the "now playing" line its stream carries);
    // listens in for up to ~25 s, so it is asked for the station that plays.
    text: (ref: string) => request<{ ref: string; text: string }>('GET', `/api/v1/radio/text?ref=${encodeURIComponent(ref)}`),
    // Radio recordings that haven't ended yet, soonest first.
    timers: () => request<{ timers: Timer[] }>('GET', '/api/v1/radio/timers'),
    picon: (ref: string, name: string) => requestBlob(`/api/v1/radio/picon?ref=${encodeURIComponent(displayRef(ref))}&name=${encodeURIComponent(name)}`),
  },
  stream: {
    // The TV bouquets (no headings).
    bouquets: () => request<{ bouquets: Bouquet[] }>('GET', '/api/v1/stream/bouquets'),
    // A bouquet's channels with now/next, and what the box shows.
    channels: (bouquet: string) =>
      request<{ channels: NowNext[]; playing: string }>('GET', `/api/v1/stream/channels?bouquet=${encodeURIComponent(bouquet)}`),
    picon: (ref: string, name: string) => requestBlob(`/api/v1/stream/picon?ref=${encodeURIComponent(displayRef(ref))}&name=${encodeURIComponent(name)}`),
    // The streams themselves are read by src/streaming/tsPlayer.ts.
    // Plays a recording of this box on another box's screen (M7); the hub
    // streams it from here.
    playOn: (target: string, path: string, title?: string) =>
      request<{ playing: string; target: string }>('POST', '/api/v1/stream/recording/play-on', { target, path, title }),
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
    // items picks exactly those items of a section (?items.<section>=…);
    // without it a section brings its default-included items.
    backup: (sections?: string[], items?: Record<string, string[]>) => {
      const params = new URLSearchParams()
      if (sections?.length) params.set('sections', sections.join(','))
      for (const [section, ids] of Object.entries(items ?? {})) params.set('items.' + section, ids.join(','))
      const q = params.toString()
      return request<BackupFile>('GET', `/api/v1/system/backup${q ? `?${q}` : ''}`)
    },
    backupSections: () => request<BackupSectionsList>('GET', '/api/v1/system/backup/sections'),
    restorePlan: (file: BackupFile) => request<{ plan: Plan }>('POST', '/api/v1/system/backup/restore', file),
    // App15: backups kept on the box. save takes the same sections as backup.
    saveBackup: (sections?: string[], items?: Record<string, string[]>) => {
      const params = new URLSearchParams()
      if (sections?.length) params.set('sections', sections.join(','))
      for (const [section, ids] of Object.entries(items ?? {})) params.set('items.' + section, ids.join(','))
      const q = params.toString()
      return request<{ file: StoredBackup }>('POST', `/api/v1/system/backup/save${q ? `?${q}` : ''}`)
    },
    storedBackups: () => request<{ dir: string; files: StoredBackup[] }>('GET', '/api/v1/system/backup/files'),
    storedBackup: (name: string) => request<BackupFile>('GET', `/api/v1/system/backup/files/${encodeURIComponent(name)}`),
    restoreStoredPlan: (name: string) => request<{ plan: Plan }>('POST', `/api/v1/system/backup/files/${encodeURIComponent(name)}/restore`),
    deleteStoredBackup: (name: string) => request<void>('DELETE', `/api/v1/system/backup/files/${encodeURIComponent(name)}`),
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
    // The package browser (docs/ADMINHUB_PLAN.md, A2); status "available"
    // is the feeds' catalog of packages that aren't installed.
    packages: (filter: { q?: string; category?: string; status?: string; lang?: string } = {}) => {
      const params = new URLSearchParams()
      if (filter.q) params.set('q', filter.q)
      if (filter.category) params.set('category', filter.category)
      if (filter.status) params.set('status', filter.status)
      if (filter.lang) params.set('lang', filter.lang)
      return request<{ packages: AdminPackage[]; categories: PackageCategory[]; truncated?: boolean }>('GET', `/api/v1/admin/packages?${params}`)
    },
    // Install, upgrade or remove as a plan: apt-get's own simulate run is
    // the preview. "upgrade" with no packages upgrades everything.
    packagesPlan: (action: 'install' | 'remove' | 'upgrade', packages: string[]) =>
      request<{ plan: Plan; steps: PackageStep[] }>('POST', '/api/v1/admin/packages/plan', { action, packages }),
    packagesLastRun: () => request<{ run: PackageRun | null }>('GET', '/api/v1/admin/packages/last-run'),
    // Downgrade: the older versions the feeds carry (usually none), then a plan.
    packageVersions: (name: string) => request<{ versions: string[] }>('GET', `/api/v1/admin/packages/${encodeURIComponent(name)}/versions`),
    downgradePlan: (name: string, version: string) =>
      request<{ plan: Plan; steps: PackageStep[] }>('POST', '/api/v1/admin/packages/downgrade/plan', { name, version }),
    // On-demand only — nothing sent unless this is actually called.
    explainPackage: (name: string, description: string) =>
      request<{ explanation: string }>('POST', '/api/v1/admin/packages/explain', { name, description }),
    feeds: () => request<{ feeds: AdminFeed[] }>('GET', '/api/v1/admin/feeds'),
    // Switch a feed on or off, or add one: preview, then apply.
    feedPlan: (action: 'enable' | 'disable' | 'add', name: string, url?: string) =>
      request<{ plan: Plan }>('POST', '/api/v1/admin/feeds/plan', { action, name, url }),
  },
  notifications: {
    list: (limit = 20) => request<{ notifications: Notification[]; unread: number }>('GET', `/api/v1/notifications?limit=${limit}`),
    markRead: () => request<{ unread: number }>('POST', '/api/v1/notifications/read'),
  },
  extensions: {
    list: () => request<{ extensions: ExtensionCard[] }>('GET', '/api/v1/extensions'),
    planAction: (id: string, actionId: string) =>
      request<{ plan: Plan }>('POST', `/api/v1/extensions/${encodeURIComponent(id)}/actions/${encodeURIComponent(actionId)}/plan`),
    settings: (id: string) => request<{ fields: ExtensionSetting[] }>('GET', `/api/v1/extensions/${encodeURIComponent(id)}/settings`),
    // Only the values the person changed; an extension keeps the rest.
    saveSettings: (id: string, values: Record<string, string>) =>
      request<{ fields: ExtensionSetting[] }>('PUT', `/api/v1/extensions/${encodeURIComponent(id)}/settings`, { values }),
  },
  apps: {
    catalog: (lang: string) => request<{ apps: CatalogApp[] }>('GET', `/api/v1/apps/catalog?lang=${encodeURIComponent(lang)}`),
    search: (q: string) => request<{ results: AppSearchResult[] }>('GET', `/api/v1/apps/search?q=${encodeURIComponent(q)}`),
    manifest: (image: string) => request<AppManifest>('GET', `/api/v1/apps/manifest?image=${encodeURIComponent(image)}`),
    installed: () => request<{ apps: InstalledApp[] }>('GET', '/api/v1/apps/installed'),
    // "Set up Docker" (backend/internal/dockersetup): its state, the last
    // run's log, and the plan for whatever's still missing.
    docker: () =>
      request<{ state: DockerSetupState; version: string; lastRun: { at: string; ok: boolean; log: string } | null }>('GET', '/api/v1/apps/docker'),
    dockerSetupPlan: () => request<{ plan: Plan; steps: DockerSetupStep[] }>('POST', '/api/v1/apps/docker/setup/plan'),
    // "Uninstall Docker": deleteData = also delete the app data (default no).
    dockerUninstallPlan: (deleteData: boolean) =>
      request<{ plan: Plan; steps: DockerSetupStep[] }>('POST', '/api/v1/apps/docker/uninstall/plan', { deleteData }),
    installPlan: (req: InstallRequest) => request<{ plan: Plan }>('POST', '/api/v1/apps/install/plan', req),
    // App15: apps whose choices are on the box without containers, and the
    // install plan that brings one back with them.
    restorable: () => request<{ apps: RestorableApp[] }>('GET', '/api/v1/apps/restorable'),
    restorablePlan: (id: string) => request<{ plan: Plan }>('POST', `/api/v1/apps/restorable/${encodeURIComponent(id)}/plan`),
    // An admin's own container (App12): turn a pasted compose file, an image
    // name or an Unraid template into the install form.
    parseCustom: (kind: 'compose' | 'image' | 'unraid', text: string, lang: string, name = '') =>
      request<{ app: ParsedApp }>('POST', '/api/v1/apps/custom/parse', { kind, text, lang, name }),
    // The disks an app can be stored on (never the box's own flash).
    storage: () => request<{ choices: AppStorageChoice[] }>('GET', '/api/v1/apps/storage'),
    removePlan: (id: string) => request<{ plan: Plan }>('POST', `/api/v1/apps/installed/${encodeURIComponent(id)}/remove/plan`),
    start: (id: string) => request<void>('POST', `/api/v1/apps/installed/${encodeURIComponent(id)}/start`),
    stop: (id: string) => request<void>('POST', `/api/v1/apps/installed/${encodeURIComponent(id)}/stop`),
    logs: (id: string) => request<{ text: string }>('GET', `/api/v1/apps/installed/${encodeURIComponent(id)}/logs`),
    // Auto-start: a direct, reversible switch.
    setAutostart: (id: string, enabled: boolean) =>
      request<{ autostart: boolean }>('PUT', `/api/v1/apps/installed/${encodeURIComponent(id)}/autostart`, { enabled }),
    // Edit (App13): the form's values, then a plan that makes the containers again.
    editForm: (id: string, lang: string) =>
      request<AppEditView>('GET', `/api/v1/apps/installed/${encodeURIComponent(id)}/edit?lang=${encodeURIComponent(lang)}`),
    editPlan: (id: string, req: InstallRequest) =>
      request<{ plan: Plan }>('POST', `/api/v1/apps/installed/${encodeURIComponent(id)}/edit/plan`, req),
    // One update notion (App14): the count, one plan over containers and
    // packages, one container's update and roll back, the kept versions.
    updates: () => request<UpdatesView>('GET', '/api/v1/apps/updates'),
    updatesPlan: () => request<UpdatePlan>('POST', '/api/v1/apps/updates/plan'),
    updatePlan: (id: string) => request<{ plan: Plan }>('POST', `/api/v1/apps/installed/${encodeURIComponent(id)}/update/plan`),
    rollbackPlan: (id: string) => request<{ plan: Plan }>('POST', `/api/v1/apps/installed/${encodeURIComponent(id)}/rollback/plan`),
    dropKeptPlan: () => request<{ plan: Plan }>('POST', '/api/v1/apps/docker/images/kept/plan'),
    // Clean up unused images: the list with sizes, then a plan.
    unusedImages: () => request<UnusedImages>('GET', '/api/v1/apps/docker/images'),
    pruneImagesPlan: () => request<{ plan: Plan }>('POST', '/api/v1/apps/docker/images/prune/plan'),
    // The console (docs/APPSHUB_PLAN.md, App5): same enable/disable shape
    // as admin.terminalStatus/enableTerminal/disableTerminal above, plus
    // a WebSocket path the component dials directly (browsers don't
    // route WebSocket connections through fetch/XHR).
    consoleStatus: () => request<AppConsoleStatus>('GET', '/api/v1/apps/console/status'),
    enableConsole: (password: string) => request<{ enabled: boolean }>('POST', '/api/v1/apps/console/enable', { password }),
    disableConsole: () => request<{ enabled: boolean }>('POST', '/api/v1/apps/console/disable'),
    consolePath: (id: string) => `/api/v1/apps/installed/${encodeURIComponent(id)}/console`,
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
