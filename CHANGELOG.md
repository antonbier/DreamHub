# Changelog

All notable changes to DreamHub are documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning follows [Semantic Versioning](https://semver.org/) (see `PRINCIPLES.md`).

## [Unreleased]

### Changed
- Concept extended: customizable tile dashboard (decided over a timeline), sidebar navigation, AdminHub (logs in v1, terminal later), roles and multiple dashboards in the data model, internationalization (DE/IT/EN, drop-in locales, RTL-ready), time zone handling (UTC inside), UX direction ("good-looking work tool", desktop-first, Source Sans 3 bundled, dark by default), LAN security (CSRF, DNS rebinding).
- Product restructured into a core (shell), hubs and connectors — see `CONCEPT.md`. `ROADMAP.md` is now organized by hub and release (Foundation / v1 / v1.1 / Later) instead of Phase 0–8.
- New principles: preview and confirm for every change (two-step `plan` → `apply` API, change log, trash instead of delete), schema-driven central settings, hub status with fix instructions, AI provider as a swappable connector, profiles-ready data model.

### Fixed
- `/api/v1/health` is now public when auth is enabled, so the TV plugin's version check works without credentials. Added tests for Basic Auth and API tokens.

### Added
- Backend scaffold (Go): `/api/v1` router with health/deviceinfo/movies endpoints, JSON error envelope, structured logging, Basic Auth + API token middleware, environment-based config, and a WebControl client with both a real HTTP/XML implementation and a mock backed by fixtures from `ARCHITECTURE.md`. Tests run entirely against the mock — no device required. Confirmed cross-compiling to a static ARM64 Linux binary.

Still pre-release — see `PROJECT_PLAN.md`, `ARCHITECTURE.md`, `PRINCIPLES.md`, `ROADMAP.md` for full context.
