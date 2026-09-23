# frontend/

Responsive web app (Vue.js + Tailwind), embedded as static assets into the Go backend via `go:embed`. Not yet started.

- Mirrors the hub structure: the core shell (home, global search, notifications, settings, onboarding) plus one routed section per hub, hidden when the hub is disabled. See [../CONCEPT.md](../CONCEPT.md).
- User-switchable language selector (i18n) — see [../PRINCIPLES.md](../PRINCIPLES.md).
- Every change goes through a preview-and-confirm dialog backed by the API's `plan` → `apply` flow.
- Not an installable PWA in v1 (service workers need HTTPS). See [../ROADMAP.md](../ROADMAP.md).
