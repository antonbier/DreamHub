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
// here, in the same three languages.
type texts struct{ title, body string }

var catalog = map[string]map[string]texts{
	"recording.started": {
		"de": {"Aufnahme gestartet", "{name} · {service}"},
		"en": {"Recording started", "{name} · {service}"},
		"it": {"Registrazione avviata", "{name} · {service}"},
	},
	"recording.finished": {
		"de": {"Aufnahme fertig", "{name} · {service}"},
		"en": {"Recording finished", "{name} · {service}"},
		"it": {"Registrazione terminata", "{name} · {service}"},
	},
	"disk.aboveThreshold": {
		"de": {"Festplatte fast voll", "{model} ist zu {percent} % belegt."},
		"en": {"Disk almost full", "{model} is {percent}% full."},
		"it": {"Disco quasi pieno", "{model} è pieno al {percent}%."},
	},
	"timer.conflict": {
		"de": {"Möglicher Timer-Konflikt", "{name} ({service}) um {time} – zu viele Aufnahmen für die Tuner."},
		"en": {"Possible timer conflict", "{name} ({service}) at {time} — more recordings than tuners."},
		"it": {"Possibile conflitto di timer", "{name} ({service}) alle {time}: più registrazioni che sintonizzatori."},
	},
	"series.missingAiring": {
		"de": {"Fehlende Folge läuft", "{series} {code} „{title}“ – {time} auf {service}"},
		"en": {"Missing episode airs", "{series} {code} “{title}” — {time} on {service}"},
		"it": {"Va in onda un episodio mancante", "{series} {code} «{title}» – {time} su {service}"},
	},
	"push.test": {
		"de": {"DreamHub", "Benachrichtigungen kommen an."},
		"en": {"DreamHub", "Notifications are arriving."},
		"it": {"DreamHub", "Le notifiche arrivano."},
	},
}

var tags = map[string][]string{
	"recording.started":    {"red_circle"},
	"recording.finished":   {"white_check_mark"},
	"disk.aboveThreshold":  {"warning"},
	"timer.conflict":       {"warning"},
	"series.missingAiring": {"tv"},
	"push.test":            {"tada"},
}

var leftover = regexp.MustCompile(`( · | \()?\{[a-z]+\}\)?`)

// Known reports whether key has a push text.
func Known(key string) bool { _, ok := catalog[key]; return ok }

// Compose builds the message for an event in language lang ("de", "en",
// "it"; anything else falls back to English). params are the event's own
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
