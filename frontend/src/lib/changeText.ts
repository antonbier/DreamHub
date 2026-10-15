// The change log keeps the English sentence the backend wrote when the
// change happened. On screen it is shown in the interface language: the
// known sentences are matched here and said again from the locale files
// (system.changes.text.*); anything else is shown as it was written.
type Translate = (key: string, params?: Record<string, unknown>) => string
type Exists = (key: string) => boolean

// [pattern, locale id, names of the captured groups]
const RULES: [RegExp, string, string[]][] = [
  [/^Updated settings: (.+)$/, 'settings', ['list']],
  [/^Created timer: (.+)$/, 'timerCreated', ['what']],
  [/^Deleted timer: (.+)$/, 'timerDeleted', ['what']],
  [/^Disabled timer: (.+)$/, 'timerDisabled', ['what']],
  [/^Enabled timer: (.+)$/, 'timerEnabled', ['what']],
  [/^Removed (\d+) finished timer\(s\)$/, 'timersCleaned', ['n']],
  [/^Restored DreamHub state from a backup file$/, 'stateRestored', []],
  [/^Enabled the terminal$/, 'terminalOn', []],
  [/^Disabled the terminal$/, 'terminalOff', []],
  [/^Enabled the Docker app console$/, 'consoleOn', []],
  [/^Disabled the Docker app console$/, 'consoleOff', []],
  [/^Set log level to (.+)$/, 'logLevel', ['level']],
  [/^Reset dashboard to the template$/, 'dashboardReset', []],
  [/^Updated dashboard layout$/, 'dashboardLayout', []],
  [/^Updated the viewers' start page$/, 'dashboardTemplate', []],
  [/^Moved (\d+) recording\(s\) to the trash$/, 'trashed', ['n']],
  [/^Restored (\d+) recording\(s\) from the trash$/, 'untrashed', ['n']],
  [/^Deleted (\d+) recording\(s\) from the trash for good/, 'emptied', ['n']],
  [/^Renamed (\d+) recording\(s\)/, 'renamed', ['n']],
  [/^Saved (\d+) cover file\(s\)/, 'covers', ['n']],
  [/^Marked (\d+) ad break\(s\)/, 'adsMarked', ['n']],
  [/^Cut (\d+) ad break\(s\)/, 'adsCut', ['n']],
  [/^Uploaded script (.+)$/, 'scriptUploaded', ['name']],
  [/^Deleted script (.+)$/, 'scriptDeleted', ['name']],
  [/^Ran? script (.+)$/, 'scriptRan', ['name']],
  [/^Created API token "(.+)"$/, 'tokenCreated', ['name']],
  [/^Revoked API token "(.+)"$/, 'tokenRevoked', ['name']],
  [/^Connected Plex server (.+?) at /, 'plexConnected', ['name']],
]

// A setting's key ("general.language") as its name on the settings page.
function settingName(key: string, t: Translate, te: Exists): string {
  const hub = /^hubs\.(\w+)\.enabled$/.exec(key)
  if (hub && te(`hub.${hub[1]}`)) return t(`hub.${hub[1]}`)
  return te(`settings.${key}`) && t(`settings.${key}`) !== `settings.${key}` ? t(`settings.${key}`) : key
}

export function changeText(summary: string, t: Translate, te: Exists): string {
  for (const [re, id, names] of RULES) {
    const m = re.exec(summary)
    if (!m) continue
    const params: Record<string, unknown> = {}
    names.forEach((n, i) => (params[n] = m[i + 1]))
    if (id === 'settings') params.list = String(params.list).split(', ').map((k) => settingName(k, t, te)).join(', ')
    return t(`system.changes.text.${id}`, params)
  }
  return summary
}
