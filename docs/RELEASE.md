# Making a release

A checklist, in order. Each step is here because skipping it went wrong once or would cut into what's on the TV.

## 1. Prepare

- [ ] `CHANGELOG.md`: move "Unreleased" under `## [<version>] - <date>`.
- [ ] Version in **both** places — they must match, `build-deb.sh` refuses otherwise:
  - `backend/internal/api/router.go`: `const Version`
  - `enigma2-plugin/src/__init__.py`: `PluginVersion`
  - and `frontend/package.json` `version` (shown nowhere critical, kept in step)
- [ ] Docs that name the current release: `README.md` ("Status"), `PROJECT_PLAN.md`, `ROADMAP.md` (header).
- [ ] Gates green locally (CI runs the same; `-race` only runs in CI otherwise):
  ```sh
  cd backend && go vet ./... && go test -race ./...
  cd ../frontend && npm run build
  ```
- [ ] Anything only tried against the mock that this release ships? Then it needs its box check first (ROADMAP.md marks these "Mock"). ChannelHub's writes in particular must not ship before their approved first try on the box.

## 2. Build

The `.deb` embeds whatever is in `backend/internal/webui/dist` at build time, and only `dist/index.html` is tracked (a placeholder). So, in **one** sequence, without `git checkout` in between:

```sh
cd frontend && npm run build && cd ..
ARCH=arm64 sh packaging/build-deb.sh
ARCH=armhf sh packaging/build-deb.sh
git checkout -- backend/internal/webui/dist/index.html   # only now
```

`build-deb.sh` stops if `index.html` names asset files that don't exist (a blank web UI on the box — happened on 2026-10-06) or if the two versions differ.

## 3. Install on the reference box

- [ ] Installing never restarts enigma2: when the TV plugin changed (`git diff <last tag> -- enigma2-plugin/`), `postinst` prints that it loads at the next enigma2 restart (check the message appears, and that nothing restarted). The update itself only restarts `dreamhub.service` and is safe any time.
- [ ] Copy and install (the box's SSH has no SFTP: `scp -O`):
  ```sh
  scp -O dist/dreamhub_<version>_arm64.deb root@<box>:/tmp/
  ssh root@<box> 'dpkg -i /tmp/dreamhub_<version>_arm64.deb'
  ```
- [ ] `curl http://<box>:8765/api/v1/health` shows the new version; the web UI loads (not blank); **System → Diagnostics → self-test** all green.

## 4. Publish

- [ ] Tag and release with both packages:
  ```sh
  git tag v<version> && git push origin v<version>
  gh release create v<version> dist/dreamhub_<version>_arm64.deb dist/dreamhub_<version>_armhf.deb \
    --title "DreamHub <version>" --notes-file <the version's CHANGELOG section>
  ```
- [ ] Close the issues the release finishes; move ROADMAP items from "Mock" to "box" where the box check happened.
