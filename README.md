# DreamHub

Modernes All-in-One Erweiterungssystem für die **Dreambox One (DreamOS AIO, Amlogic S922X)**.

Vereint Media-Management, EPG-Suche, TV-Steuerung und optionale KI-Empfehlungen in einer schnellen Weboberfläche — als Ergänzung, nicht als Ersatz für das native WebControl (Port 80).

## Tech-Stack

- **Backend:** Go, statisch kompiliert für ARM64. Läuft auf Port 8080. REST-API + WebSockets.
- **Frontend:** PWA (Vue.js + Tailwind), via `go:embed` in die Go-Binary eingebettet.
- **TV-Client:** minimales Enigma2-Plugin (Py2/Py3-kompatibel), rein anzeigend, ruft nur `127.0.0.1:8080` ab.
- **Distribution (Ziel):** installierbares `.ipk`-Paket über einen eigenen Feed — kein manuell platziertes Binary.

## Warum überhaupt

Alles, was "App-artig" ist (Fotos, Musik, Dashboards), macht Android TV auf dem Fernseher ohnehin besser. Sinnvoll ist nur, was **nur die Dreambox kann**, weil sie den einzigen Tuner im Haus hat: Live-TV, EPG, Aufnahmen, Timer. Das aktuelle native WebControl ist funktional, aber optisch und funktional veraltet — DreamHub soll das ersetzen, ohne den Enigma2-Kern anzufassen.

## Status

Frühe Planungsphase. Siehe [ROADMAP.md](ROADMAP.md) für Module, Phasen und offene technische Fragen.

## Nicht im Scope

- **Space-Saver / Transcoding auf Unraid:** verworfen, weil der Unraid-Server nicht 24/7 läuft. Wird stattdessen als separates, unabhängiges Skript gelöst, das fertige Releases über den Arr-Stack nachlädt — kein Live-Feature, keine Abhängigkeit von DreamHub.
- **Home Assistant Integration:** nicht relevant, wird nicht genutzt.
- **Casting auf andere Geräte:** bereits durch DreamEPG Premium gelöst.
