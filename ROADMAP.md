# Roadmap

## Phase 0 — Spikes (vor dem eigentlichen Bau klären)

Diese vier Fragen entscheiden über die Machbarkeit einzelner Module und sollten zuerst beantwortet werden.

- [ ] **Bouqueteditor-API prüfen** — WebControl-Bouqueteditor im Browser öffnen, Devtools → Network beim Verschieben/Speichern beobachten. Gibt es einen wiederverwendbaren lokalen Endpoint (Port 80), oder muss DreamHub Bouquet-Dateien selbst parsen/schreiben?
- [ ] **Hardware-Transcoding verifizieren** — Addon-/Plugin-Manager der Box prüfen: wird ein "Transcoding"-Plugin für die Dreambox One überhaupt angeboten? (S922X ist laut Datenblatt ein reiner Decoder-Chip ohne Hardware-Encoder — vermutlich nein.)
- [ ] **ffmpeg-Verfügbarkeit** — Ist ffmpeg auf der Box vorinstalliert, oder muss es separat für ARM64/DreamOS bereitgestellt werden (für den Lossless-Trimmer in FilmHub)?
- [ ] **Sidecar-Dateien einer Aufnahme vollständig auflisten** — `.ts`, `.ts.meta`, `.ts.ap`, `.ts.sc`, `.ts.cuts`, ggf. `.eit` auf der aktuellen Firmware-Version gegenprüfen, damit die Rename-Engine nichts vergisst.

## Phase 1 — Base Engine

Go-Backend (Port 8080) mit minimal eingebettetem Frontend. Dateisystem-Scanner für `/media/hdd/movie/`, liest `.ts.meta`, stellt Ergebnisse über REST-API bereit.

## Phase 2 — FilmHub

- Grid-/Listenansicht für Aufnahmen.
- Custom-Naming-Engine inkl. **aller** Sidecar-Dateien (nicht nur `.ts.meta` — siehe Phase 0).
- Bulk Actions über Background-Queue.
- Lossless Trimmer via `ffmpeg -c copy` (Hinweis: Schnitt ist keyframe-genau, nicht frame-genau).

## Phase 3 — AIO Universal Search

- Quellen: lokale Aufnahmen, Timer/AutoTimer, SerienRecorder-Datenbank (bestätigt: SQLite unter `/etc/enigma2/SerienRecorder.db`).
- EPG-Daten **nicht** durch eigenes Parsen von `epg.dat` gewinnen (undokumentiertes Binärformat, bricht bei Firmware-Updates) — stattdessen die bestehende lokale API von WebControl/Enigma2 anfragen.

## Phase 4 — TV Control & WebAdmin (modernisierte Fassade)

- Bestehende WebControl-Funktionen (OSD-Screenshot, System-Message, Power-Menü, Live-Metriken) mit moderner UI neu bauen — Logik existiert bereits, hier geht es nur um die Oberfläche.
- **Grid-EPG ("TV-Zeitschrift")**: tabellarische Mehrkanal-Übersicht. Online-/KI-Anreicherung (TMDB o. ä.) **nur** für Film-/Serien-Genres — bei Tagesformaten (Nachrichten, Magazine) bringt Matching nichts.
- **Bouqueteditor**: Umsetzung hängt von Spike-Ergebnis aus Phase 0 ab.

## Phase 5 — Live-Streaming Bridge

- Kein Software-Transcoding (S922X hat keinen Hardware-Encoder → wäre CPU-intensiv).
- HTML5-Inline-Player nur für Sender mit AAC-kompatiblem Audio.
- Für alle anderen Sender (AC3-Audio): Fallback auf **"In VLC öffnen"** (`.m3u`-Link an native VLC-App), da VLC eigene Audio-Decoder mitbringt und der Browser-Codec-Beschränkung (kein AC3 im HTML5-`<video>`) so ausweicht.
- Kein In-Page-Embedding via Browser-Plugin (NPAPI/VLC-Plugin und Chromes "VXG" sind beide tot/veraltet).

## Phase 6 — AI Discovery

- Tagesaktuelles EPG (gefiltert nach Nutzerauswahl relevanter Sender in den Settings — Relevanz, nicht primär Kosten) an LLM-API senden, Empfehlung als OSD auf Fernbedienungs-Tastendruck.
- Grobe Kostenschätzung: ~30 Sender × ~20 Sendungen/Tag × ~150 Tokens ≈ 90k Tokens/Tag — unkritisch im Cent-Bereich, Sender-Filter dient primär der Empfehlungsqualität.

## Phase 7 — Plugin & Script Hub

- Kein tiefer Enigma2-Hook für Events (zu aufwändig für v1).
- Stattdessen: Go-Backend pollt `/api/timerlist` alle 30–60s und erkennt Statuswechsel (z. B. "Aufnahme beendet") selbst — ausreichend für Skript-Trigger.
- Trennung: `scripts/` (einfache Bash/Python-Hooks) vs. `plugins/` (fest eingebundene Backend-Routen mit eigenem Settings-Tab).

## Phase 8 — Packaging

Ziel-Zustand: `.ipk`-Paket über einen eigenen opkg-Feed installierbar, statt manuell per Telnet platziertem Binary. Risiko durch fehlende Kern-Firmware-Updates (Hersteller insolvent) ohnehin gering, sauberes Packaging bleibt trotzdem das Ziel für Wartbarkeit/Updates.
