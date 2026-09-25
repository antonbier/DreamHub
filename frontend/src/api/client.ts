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
import { authHeader, authState, clearCredentials } from './authState'
import type {
  ApiToken,
  AutoTimers,
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
  HealthInfo,
  HubStatus,
  LogEntry,
  LogLevel,
  Navigation,
  Notification,
  Plan,
  Recommendation,
  SearchResult,
  SeriesMarker,
  SettingsView,
  Timer,
  UpcomingEpisode,
  WhoAmI,
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

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {}
  if (method !== 'GET' && method !== 'HEAD') {
    headers[CSRF_HEADER] = '1'
  }
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
  }
  const auth = authHeader()
  if (auth) headers['Authorization'] = auth

  let res: Response
  try {
    res = await fetch(path, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    })
  } catch (err) {
    // Network failure (backend unreachable) — no HTTP response at all.
    throw new ApiError(0, 'network_error', err instanceof Error ? err.message : String(err))
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
    // absent credentials — either way, the login screen decides next.
    clearCredentials()
    authState.authenticated = false
  }

  if (!res.ok) {
    const envelope = (json as { error?: { code?: string; message?: string; details?: ApiError['details'] } })?.error
    throw new ApiError(
      res.status,
      envelope?.code ?? 'unknown',
      envelope?.message ?? res.statusText,
      requestId,
      envelope?.details,
    )
  }
  return json as T
}

export const api = {
  health: () => request<HealthInfo>('GET', '/api/v1/health'),
  whoami: () => request<WhoAmI>('GET', '/api/v1/whoami'),
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
    folder: (dir?: string) =>
      request<MediaFolder>('GET', `/api/v1/media/movies${dir ? `?dir=${encodeURIComponent(dir)}` : ''}`),
    suggestName: (path: string, source: SuggestionSource) =>
      request<{ suggestions: NameSuggestion[] }>('POST', '/api/v1/media/rename/suggest', { path, source }),
    renamePlan: (path: string, newName: string) => request<{ plan: Plan }>('POST', '/api/v1/media/rename/plan', { path, newName }),
    trashPlan: (paths: string[]) => request<{ plan: Plan }>('POST', '/api/v1/media/trash/plan', { paths }),
    trash: () => request<{ items: TrashItem[] }>('GET', '/api/v1/media/trash'),
    restorePlan: (paths: string[]) => request<{ plan: Plan }>('POST', '/api/v1/media/trash/restore/plan', { paths }),
    emptyPlan: (paths: string[]) => request<{ plan: Plan; bytes: number }>('POST', '/api/v1/media/trash/empty/plan', { paths }),
    disk: () => request<{ disks: Disk[] }>('GET', '/api/v1/media/disk'),
  },
  guide: {
    bouquets: () => request<{ bouquets: Bouquet[] }>('GET', '/api/v1/guide/bouquets'),
    grid: (bouquet: string, start: number, end: number) =>
      request<{ channels: ChannelSchedule[] }>(
        'GET',
        `/api/v1/guide/grid?bouquet=${encodeURIComponent(bouquet)}&start=${start}&end=${end}`,
      ),
  },
  series: {
    markers: () => request<{ markers: SeriesMarker[] }>('GET', '/api/v1/series/markers'),
    upcoming: () => request<{ episodes: UpcomingEpisode[] }>('GET', '/api/v1/series/upcoming'),
  },
  discovery: {
    status: () => request<DiscoveryStatus>('GET', '/api/v1/discovery/status'),
    generate: (bouquet: string) =>
      request<DiscoveryStatus & { recommendations: Recommendation[] }>('POST', '/api/v1/discovery/recommendations', { bouquet }),
  },
  dashboard: {
    get: () => request<DashboardLayout>('GET', '/api/v1/dashboard'),
    save: (layout: DashboardLayout) => request<DashboardLayout>('PUT', '/api/v1/dashboard', layout),
  },
  search: (query: string) =>
    request<{ results: SearchResult[] }>('GET', `/api/v1/search?q=${encodeURIComponent(query)}`),
  tokens: {
    list: () => request<{ tokens: ApiToken[] }>('GET', '/api/v1/tokens'),
    create: (name: string) => request<CreatedApiToken>('POST', '/api/v1/tokens', { name }),
    revoke: (id: number) => request<void>('DELETE', `/api/v1/tokens/${id}`),
  },
  changes: (limit = 20) => request<{ changes: ChangeEntry[] }>('GET', `/api/v1/changes?limit=${limit}`),
  system: {
    backup: () => request<BackupFile>('GET', '/api/v1/system/backup'),
    restorePlan: (file: BackupFile) => request<{ plan: Plan }>('POST', '/api/v1/system/backup/restore', file),
  },
  admin: {
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
  },
  notifications: {
    list: (limit = 20) => request<{ notifications: Notification[]; unread: number }>('GET', `/api/v1/notifications?limit=${limit}`),
    markRead: () => request<{ unread: number }>('POST', '/api/v1/notifications/read'),
  },
  control: {
    // A plain path, not a fetch() call — the screenshot is rendered
    // directly as <img :src>, with a cache-busting query param.
    screenshotPath: () => `/api/v1/control/screenshot?t=${Date.now()}`,
    power: () => request<{ inStandby: boolean }>('GET', '/api/v1/control/power'),
    sendKey: (code: number) => request<{ sent: number }>('POST', '/api/v1/control/key', { code }),
    zap: (serviceReference: string) => request<{ zapped: string }>('POST', '/api/v1/control/zap', { serviceReference }),
  },
}
