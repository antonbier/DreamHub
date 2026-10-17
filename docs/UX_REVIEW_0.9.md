# UX review before external testers (2026-10-16, 0.9.0, mock mode)

Blind review by an agent as a first-time tester: every page and Settings tab, phone (390×844) and desktop (1280×800), en and de (it only by key parity), then a second box added through the UI. Collected, not fixed — except the bugs marked **fixed**. Screenshots were kept in the session scratchpad only.

Not seen live (the mock can't): pairing, "Play on <box>", a box going offline (greyed selector, strip wording).

## Fixed in the bug run
- A1 the setup wizard never appeared on a fresh install (hidden `apps.autostartBoot` counted as "configured") — hidden fields no longer count.
- A2 hidden bookkeeping keys (`general.setupDone`, `apps.autostartBoot`) no longer land in the change log.
- B1 phone header overflowed with two boxes (menu unreachable) — tighter gaps, box name truncated.
- G1 the API text showed `POST /api/v1/plans//apply` (vue-i18n swallowed `{id}`).
- C7 "Colour"/"Color"; C9 wrong tab names in hints (Authentication→Access, AI/TMDB→Services); D4 `mr-1.5`→`me-1.5`; F6 "Sie/Ihre" in German → du.

## Fixed after the review
- C1 one box: no Boxes tab (an "Another box" card in General leads there), developer block collapsed.
- D1/D2 a Basic box's deep link and Device page say what needs DreamHub on the box; B4's "Apps" row no longer counts under Needs attention.
- E1 Needs attention counts only real trouble, the TMDB sentence is said once on the dashboard (with a link).
- Bug-run notes below: all four fixed (Docker "Open" link has an address setting, poller back-off 1 min, job events filtered by box list, `user:pw@` kept).

## Open — decide before the testers get it
- **B2/B3** The "other boxes" strip is an unlabelled chip; nothing on a page says which box is shown except the header pill. Label the strip, show the box in the page title/colour.
- **B4** Second box's dashboard still shows the first box's "Upcoming episodes" and the extension card.
- **C2/C3** Pairing wording ("DreamHub" ×3 for two machines), Edit card of a Basic box holds two forms, a disclosure labelled with an error-like sentence. "Get a code" + numbered steps.
- **E2** "Settings" in a page banner's fix text ("Configure it in Settings → Plex") is not a link.
- **F1/F2** Extensions tab has no install path; "Packages / Extensions / Docker" — rename to System software / Extensions / Apps.
- **H1** "Device/Gerät" = the box's admin page, "this device" = the browser. Ambiguous with two boxes.
- **E3** Timers: no hint how to create one; destructive "Clean up" next to an empty state.
- **A3** Wizard switches have no accessible name (`OnboardingHubsStep`, `OnboardingAuthStep`).

## Open — minor
- A4 wizard card vertically centred, buttons jump; A5 no pointer to a second box.
- B5 selector menu flush to the screen edge on phone; B6 first box is named "Dreambox" with no colour; B7 header icons 32 px.
- C4 add-box form: placeholder looks real, greyed buttons without reason, narrow login fields; C5 power-on block heavy for a first add (optional disclosure); C6 box rows show no state/Test; C8 profile editor: all boxes unticked reads as "none" (default all ticked).
- D3 Basic-box note styled as body text; D4 "Play on the TV" next to "Play on <box>" is ambiguous (from code).
- E4 AutoTimer jargon; E5 Radio "Play on the box" looks disabled, truncated names; E6 Live TV "Watch what the TV shows" unexplained, duplicate "Pick a channel"; E7 Recommendations dead end; E8 Services: one-click Restart on enigma2/dreamhub (confirm), raw daemon list; E9 tab labels cut on phone ("Advanc…", "Services"); E10 ragged desktop dashboard; E11 icon-only remote button.
- F3 unlabelled gear next to every Apps tab sentence; F4 Packages offers Remove on `enigma2`; F5 "…" menu small.
- G2 secret fields all say "New key"; G3 Access shows Username/Password while sign-in is off; G4 "WebControl"/"ntfy" as headings; G5 backup buttons wrap.
- H2 German mixes Box/Dreambox/Gerät; H3 truncated date in Upcoming episodes.
- Tap targets under 44 px in many places (tabs 39, inputs 35, dialog Close 28, remote buttons 28); check contrast of `--dh-faint` on `--dh-tile`.

## Bug-run notes (not UX)
All four found in the bug run are fixed — see "Fixed after the review".
