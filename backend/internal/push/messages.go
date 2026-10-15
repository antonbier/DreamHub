package push

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// The texts a push carries, per event key and language. The web UI
// translates notifications itself (frontend locales, "notification.*");
// a push leaves the box as finished text, so the few it sends are kept
// here, in the same languages.
type texts struct{ title, body string }

var catalog = map[string]map[string]texts{
	"recording.started": {
		"de": {"Aufnahme gestartet", "{name} · {service}"},
		"en": {"Recording started", "{name} · {service}"},
		"it": {"Registrazione avviata", "{name} · {service}"},
		"fr": {"Enregistrement démarré", "{name} · {service}"},
		"es": {"Grabación iniciada", "{name} · {service}"},
		"nl": {"Opname gestart", "{name} · {service}"},
		"pl": {"Nagrywanie rozpoczęte", "{name} · {service}"},
		"tr": {"Kayıt başladı", "{name} · {service}"},
	},
	"recording.finished": {
		"de": {"Aufnahme fertig", "{name} · {service}"},
		"en": {"Recording finished", "{name} · {service}"},
		"it": {"Registrazione terminata", "{name} · {service}"},
		"fr": {"Enregistrement terminé", "{name} · {service}"},
		"es": {"Grabación terminada", "{name} · {service}"},
		"nl": {"Opname klaar", "{name} · {service}"},
		"pl": {"Nagrywanie zakończone", "{name} · {service}"},
		"tr": {"Kayıt bitti", "{name} · {service}"},
	},
	"disk.aboveThreshold": {
		"de": {"Festplatte fast voll", "{model} ist zu {percent} % belegt."},
		"en": {"Disk almost full", "{model} is {percent}% full."},
		"it": {"Disco quasi pieno", "{model} è pieno al {percent}%."},
		"fr": {"Disque presque plein", "{model} est plein à {percent} %."},
		"es": {"Disco casi lleno", "{model} está lleno al {percent} %."},
		"nl": {"Schijf bijna vol", "{model} is voor {percent}% vol."},
		"pl": {"Dysk prawie pełny", "{model} jest zapełniony w {percent}%."},
		"tr": {"Disk neredeyse dolu", "{model} %{percent} dolu."},
	},
	"timer.conflict": {
		"de": {"Möglicher Timer-Konflikt", "{name} ({service}) um {time} – zu viele Aufnahmen für die Tuner."},
		"en": {"Possible timer conflict", "{name} ({service}) at {time} — more recordings than tuners."},
		"it": {"Possibile conflitto di timer", "{name} ({service}) alle {time}: più registrazioni che sintonizzatori."},
		"fr": {"Conflit de minuteries possible", "{name} ({service}) à {time} — plus d’enregistrements que de tuners."},
		"es": {"Posible conflicto de temporizadores", "{name} ({service}) a las {time}: más grabaciones que sintonizadores."},
		"nl": {"Mogelijk timerconflict", "{name} ({service}) om {time} — meer opnames dan tuners."},
		"pl": {"Możliwy konflikt timerów", "{name} ({service}) o {time} — więcej nagrań niż tunerów."},
		"tr": {"Olası zamanlayıcı çakışması", "{name} ({service}) {time} — kayıt sayısı tuner sayısından fazla."},
	},
	"series.missingAiring": {
		"de": {"Fehlende Folge läuft", "{series} {code} „{title}“ – {time} auf {service}"},
		"en": {"Missing episode airs", "{series} {code} “{title}” — {time} on {service}"},
		"it": {"Va in onda un episodio mancante", "{series} {code} «{title}» – {time} su {service}"},
		"fr": {"Un épisode manquant est diffusé", "{series} {code} « {title} » – {time} sur {service}"},
		"es": {"Se emite un episodio que falta", "{series} {code} «{title}» – {time} en {service}"},
		"nl": {"Ontbrekende aflevering wordt uitgezonden", "{series} {code} ‘{title}’ – {time} op {service}"},
		"pl": {"Leci brakujący odcinek", "{series} {code} „{title}” – {time} na {service}"},
		"tr": {"Eksik bölüm yayında", "{series} {code} “{title}” – {time}, {service}"},
	},
	"service.failed": {
		"de": {"Dienst ausgefallen", "{name} auf der Box läuft nicht mehr."},
		"en": {"Service down", "{name} on the box stopped running."},
		"it": {"Servizio fermo", "{name} sul box non è più in esecuzione."},
		"fr": {"Service arrêté", "{name} ne tourne plus sur le récepteur."},
		"es": {"Servicio detenido", "{name} ha dejado de funcionar en el receptor."},
		"nl": {"Dienst uitgevallen", "{name} op de box draait niet meer."},
		"pl": {"Usługa zatrzymana", "{name} na odbiorniku przestała działać."},
		"tr": {"Servis durdu", "Kutudaki {name} artık çalışmıyor."},
	},
	"service.recovered": {
		"de": {"Dienst läuft wieder", "{name} auf der Box läuft wieder."},
		"en": {"Service back", "{name} on the box is running again."},
		"it": {"Servizio ripristinato", "{name} sul box è di nuovo in esecuzione."},
		"fr": {"Service rétabli", "{name} tourne de nouveau sur le récepteur."},
		"es": {"Servicio restablecido", "{name} vuelve a funcionar en el receptor."},
		"nl": {"Dienst draait weer", "{name} op de box draait weer."},
		"pl": {"Usługa przywrócona", "{name} na odbiorniku znowu działa."},
		"tr": {"Servis yeniden çalışıyor", "Kutudaki {name} yeniden çalışıyor."},
	},
	"package.upgradable": {
		"de": {"Updates verfügbar", "{count} Paket(e) auf der Box können aktualisiert werden."},
		"en": {"Updates available", "{count} package(s) on the box can be updated."},
		"it": {"Aggiornamenti disponibili", "{count} pacchetto/i sul box possono essere aggiornati."},
		"fr": {"Mises à jour disponibles", "{count} paquet(s) sur le récepteur peuvent être mis à jour."},
		"es": {"Actualizaciones disponibles", "{count} paquete(s) en el receptor se pueden actualizar."},
		"nl": {"Updates beschikbaar", "{count} pakket(ten) op de box kunnen worden bijgewerkt."},
		"pl": {"Dostępne aktualizacje", "{count} pakiet(y) na odbiorniku można zaktualizować."},
		"tr": {"Güncellemeler mevcut", "Kutudaki {count} paket güncellenebilir."},
	},
	"app.upgradable": {
		"de": {"App-Update verfügbar", "Für {name} ist ein neues Image verfügbar."},
		"en": {"App update available", "A newer image is available for {name}."},
		"it": {"Aggiornamento app disponibile", "È disponibile una nuova immagine per {name}."},
		"fr": {"Mise à jour d’appli disponible", "Une nouvelle image est disponible pour {name}."},
		"es": {"Actualización de app disponible", "Hay una nueva imagen disponible para {name}."},
		"nl": {"App-update beschikbaar", "Er is een nieuwere image beschikbaar voor {name}."},
		"pl": {"Dostępna aktualizacja aplikacji", "Dla {name} dostępny jest nowszy obraz."},
		"tr": {"Uygulama güncellemesi mevcut", "{name} için daha yeni bir imaj mevcut."},
	},
	"updates.applied": {
		"de": {"Updates installiert", "{count} Update(s) wurden automatisch installiert: {names}."},
		"en": {"Updates installed", "{count} update(s) were installed automatically: {names}."},
		"it": {"Aggiornamenti installati", "{count} aggiornamento/i installato/i automaticamente: {names}."},
		"fr": {"Mises à jour installées", "{count} mise(s) à jour installée(s) automatiquement : {names}."},
		"es": {"Actualizaciones instaladas", "{count} actualización(es) instalada(s) automáticamente: {names}."},
		"nl": {"Updates geïnstalleerd", "{count} update(s) automatisch geïnstalleerd: {names}."},
		"pl": {"Zainstalowano aktualizacje", "Automatycznie zainstalowano aktualizacje ({count}): {names}."},
		"tr": {"Güncellemeler yüklendi", "{count} güncelleme otomatik olarak yüklendi: {names}."},
	},
	"updates.failed": {
		"de": {"Automatisches Update fehlgeschlagen", "{name}: {reason}"},
		"en": {"Automatic update failed", "{name}: {reason}"},
		"it": {"Aggiornamento automatico non riuscito", "{name}: {reason}"},
		"fr": {"Échec de la mise à jour automatique", "{name} : {reason}"},
		"es": {"Falló la actualización automática", "{name}: {reason}"},
		"nl": {"Automatische update mislukt", "{name}: {reason}"},
		"pl": {"Automatyczna aktualizacja nie powiodła się", "{name}: {reason}"},
		"tr": {"Otomatik güncelleme başarısız", "{name}: {reason}"},
	},
	"updates.rolledBack": {
		"de": {"Update zurückgenommen", "{name} lief nach dem Update nicht ({reason}). Die vorherige Version läuft wieder."},
		"en": {"Update rolled back", "{name} did not run after the update ({reason}). The previous version is running again."},
		"it": {"Aggiornamento annullato", "{name} non funzionava dopo l’aggiornamento ({reason}). La versione precedente è di nuovo in esecuzione."},
		"fr": {"Mise à jour annulée", "{name} ne fonctionnait pas après la mise à jour ({reason}). L’ancienne version tourne de nouveau."},
		"es": {"Actualización revertida", "{name} no funcionó tras la actualización ({reason}). La versión anterior vuelve a ejecutarse."},
		"nl": {"Update teruggedraaid", "{name} werkte niet na de update ({reason}). De vorige versie draait weer."},
		"pl": {"Aktualizacja cofnięta", "{name} nie działał po aktualizacji ({reason}). Poprzednia wersja znów działa."},
		"tr": {"Güncelleme geri alındı", "{name} güncellemeden sonra çalışmadı ({reason}). Önceki sürüm yeniden çalışıyor."},
	},
	"app.crashLoop": {
		"de": {"{name} hält immer wieder an", "Siehe die Logs der App."},
		"en": {"{name} keeps stopping", "See the app’s logs."},
		"it": {"{name} continua a fermarsi", "Vedi i log dell’app."},
		"fr": {"{name} s’arrête sans cesse", "Voir les journaux de l’appli."},
		"es": {"{name} se detiene una y otra vez", "Consulta los registros de la app."},
		"nl": {"{name} stopt steeds weer", "Bekijk de logboeken van de app."},
		"pl": {"{name} wciąż się zatrzymuje", "Zobacz dzienniki aplikacji."},
		"tr": {"{name} sürekli duruyor", "Uygulamanın günlüklerine bakın."},
	},
	"push.test": {
		"de": {"DreamHub", "Benachrichtigungen kommen an."},
		"en": {"DreamHub", "Notifications are arriving."},
		"it": {"DreamHub", "Le notifiche arrivano."},
		"fr": {"DreamHub", "Les notifications arrivent."},
		"es": {"DreamHub", "Las notificaciones llegan."},
		"nl": {"DreamHub", "Meldingen komen aan."},
		"pl": {"DreamHub", "Powiadomienia docierają."},
		"tr": {"DreamHub", "Bildirimler geliyor."},
	},
}

var tags = map[string][]string{
	"recording.started":    {"red_circle"},
	"recording.finished":   {"white_check_mark"},
	"disk.aboveThreshold":  {"warning"},
	"timer.conflict":       {"warning"},
	"series.missingAiring": {"tv"},
	"service.failed":       {"rotating_light"},
	"service.recovered":    {"white_check_mark"},
	"package.upgradable":   {"arrows_counterclockwise"},
	"app.upgradable":       {"arrows_counterclockwise"},
	"updates.applied":      {"white_check_mark"},
	"updates.failed":       {"warning"},
	"updates.rolledBack":   {"warning"},
	"app.crashLoop":        {"rotating_light"},
	"push.test":            {"tada"},
}

var leftover = regexp.MustCompile(`( · | \()?\{[a-z]+\}\)?`)

// Known reports whether key has a push text.
func Known(key string) bool { _, ok := catalog[key]; return ok }

// Compose builds the message for an event in language lang (one of the
// UI's languages; anything else falls back to English). params are the event's own
// (a "begin"/"start" Unix time becomes {time}, formatted in loc).
func Compose(key string, params json.RawMessage, lang string, loc *time.Location) (Message, bool) {
	byLang, ok := catalog[key]
	if !ok {
		return Message{}, false
	}
	tx, ok := byLang[lang]
	if !ok {
		tx = byLang["en"]
	}
	var p map[string]any
	_ = json.Unmarshal(params, &p)
	if loc == nil {
		loc = time.Local
	}
	for _, k := range []string{"begin", "start"} {
		if v, ok := p[k].(float64); ok && v > 0 {
			t := time.Unix(int64(v), 0).In(loc)
			layout := "02.01. 15:04"
			if lang == "en" {
				layout = "Jan 2, 15:04"
			}
			p["time"] = t.Format(layout)
		}
	}
	fill := func(s string) string {
		for k, v := range p {
			var text string
			switch v := v.(type) {
			case float64:
				text = fmt.Sprintf("%g", v)
			default:
				text = fmt.Sprint(v)
			}
			s = strings.ReplaceAll(s, "{"+k+"}", text)
		}
		return s
	}
	// A placeholder the event didn't carry goes, with the separator before it.
	clean := func(s string) string {
		s = leftover.ReplaceAllString(s, "")
		return strings.TrimSpace(s)
	}
	m := Message{Title: clean(fill(tx.title)), Body: clean(fill(tx.body)), Tags: tags[key]}
	if key == "disk.aboveThreshold" || key == "timer.conflict" {
		m.Priority = 4
	}
	return m, true
}
