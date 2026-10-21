# DreamHub Benutzerhandbuch

DreamHub ist eine Web-Oberfläche für **Dreamboxen mit DreamOS** – entwickelt und getestet auf einer Dreambox One; siehe *Was du benötigst* für andere Modelle. Es läuft direkt auf der Box, neben dem eigenen WebControl der Box, und wird über einen Browser genutzt – auf deinem Smartphone, Tablet oder Computer. Dieses Handbuch deckt die Installation, die ersten Schritte und die Funktionen der einzelnen Bereiche ab.

DreamHub ändert von sich aus nichts: Jede Änderung an Aufnahmen, Timern oder Einstellungen auf der Box wird dir zuerst angezeigt („Vorschau · noch nichts geändert“) und wird erst ausgeführt, wenn du sie bestätigst. Jede angewendete Änderung wird unter **Gerät → Übersicht → Änderungsprotokoll** aufgelistet.

---

## 1. Installation

### Was du benötigst

* Eine Dreambox mit DreamOS:
* **Dreambox One oder Two** (ARM64) – Paket `dreamhub_<version>_arm64.deb`. Entwickelt und getestet auf einer Dreambox One mit DreamOS 2.6 (Gemini/AIO Image).
* **DM900 UHD oder DM920 UHD** (ARMv7) – Paket `dreamhub_<version>_armhf.deb`. Auf dieselbe Weise gebaut, aber bisher nicht auf einer solchen Box getestet: Feedback ist willkommen.
* MIPS-Boxen (DM7080, DM820, DM520, DM525) werden derzeit noch nicht unterstützt, ältere Dreamboxen ohne DreamOS überhaupt nicht.


* Die Box in deinem Heimnetzwerk. DreamHub nutzt Port **8765**; WebControl bleibt auf Port 80.
* Ca. 15 MB Speicherplatz auf der Root-Partition für das Programm; seine Datenbank liegt auf `/data`.

### Aktuell: Manuelle Installation des Pakets

Da es noch keinen Paket-Feed gibt (siehe unten *Später: Über einen Feed*), wird das `.deb`-Paket vorerst direkt installiert.

1. **Lade das Paket** für deine Box aus dem neuesten Release herunter: [github.com/antonbier/DreamHub/releases](https://github.com/antonbier/DreamHub/releases) – `…_arm64.deb` für eine Dreambox One/Two, `…_armhf.deb` für eine DM900/DM920. (Zwischen-Builds jeder Änderung findest du im Repository unter dem Tab **Actions**, Artefakt **`dreamhub-deb`**, für jeden, der bei GitHub angemeldet ist.)
2. **Kopiere es auf die Box**, zum Beispiel nach `/tmp/` – über die Netzwerkfreigabe der Box (Samba), mit einem SCP-Programm wie WinSCP oder Cyberduck (Benutzer `root`) oder von einem USB-Stick.
3. **Installiere es.** Öffne ein Terminal (Shell) auf der Box. Es ist kein SSH-Client erforderlich: Das eigene **WebAdmin** der Box bietet ein Terminal im Browser unter `http://<box>/webadmin/terminal/` (Anmeldung als `root`). Führe dann aus:
```sh
dpkg -i /tmp/dreamhub_*.deb

```


Falls Pakete fehlen, hole sie mit `apt-get -f install` nach.

Dadurch wird der DreamHub-Dienst installiert und gestartet sowie das **DreamHub**-Plugin zur Plugin-Liste des TVs hinzugefügt. Die Benutzeroberfläche des TVs (enigma2) wird bei der Installation **nie neu gestartet**: Das Plugin erscheint beim nächsten Neustart von enigma2 (die Meldung am Ende der Installation weist darauf hin), den du machst, wenn es dir passt – DreamHub selbst läuft sofort im Browser.
4. **Überprüfe es:** Öffne DreamHub (nächster Abschnitt), gehe dann zu **Gerät → Diagnose → Selbsttest ausführen**. Es fragt die Box nach allem ab, was DreamHub benötigt, und sollte durchgehend grün anzeigen.

### Später: Über einen Feed

Sobald DreamHub in einem eigenen Paket-Feed veröffentlicht wird, funktionieren Installation und Updates ohne manuelles Kopieren von Dateien: Nutze in der Feed-Verwaltung von WebAdmin **„Neues *-feed.list hochladen“**, um die Feed-Datei von DreamHub hinzuzufügen, und installiere `dreamhub` dann wie jedes andere Plugin. Dieses Handbuch wird die Feed-Datei nennen, sobald sie existiert.

### Aktualisieren

Installiere das neuere `.deb` auf dieselbe Weise (`dpkg -i …`). Deine Einstellungen, Schlüssel und dein Dashboard bleiben erhalten: Sie liegen in `/data/dreamhub`, was von einem Update niemals angerührt wird.

### Deinstallieren

```sh
dpkg -r dreamhub      # entfernt das Programm, behält deine Einstellungen in /data/dreamhub
dpkg -P dreamhub      # löscht auch /data/dreamhub — Einstellungen, Schlüssel, Änderungsprotokoll

```

---

## 2. Erste Schritte

### DreamHub öffnen

Jede dieser Optionen funktioniert:

* **Vom Fernseher aus:** Öffne die Plugin-Liste (oder das blaue Erweiterungen-Menü), wähle **DreamHub** und scanne den QR-Code mit deinem Smartphone.
* **Über den Namen:** `http://<box-name>.local:8765` – die Box meldet sich selbst im Netzwerk an (mDNS). Die meisten Smartphones und Computer finden sie auf diese Weise.
* **Über die IP-Adresse:** `http://<box-IP>:8765`, z. B. `[http://192.168.1.43:8765](http://192.168.1.43:8765)`.

Tipp: Füge es zum Startbildschirm deines Smartphones hinzu – es öffnet sich dann wie eine App. Siehe [Abschnitt 4](https://www.google.com/search?q=%234-dreamhub-als-app).

### Der Einrichtungsassistent

Beim ersten Aufruf startet ein kurzer Assistent: Sprache, Verbindung zur Box, ein Passwort für DreamHub, welche Hubs angezeigt werden sollen, deine Senderliste und optional ein KI-Schlüssel. Alles darin kann später unter **Einstellungen** geändert werden, und **Einstellungen → System → Einrichtungsassistent ausführen** startet ihn erneut. Die Einstellungen sind in Tabs unterteilt – Allgemein, Aufnahmen, Dienste, Benachrichtigungen, Hubs, Zugriff, System – und jede Einstellung kann mit **Strg+K** gefunden werden.

### Profile: Eines pro Person

Unter **Einstellungen → Profile** legt ein Admin ein Profil für jede Person im Haushalt an. Jedes Gerät fragt einmal „Wer schaut gerade?“ und merkt sich die Antwort; ein Profil kann eine PIN haben (Admins benötigen eine, sobald es mehr als ein Profil gibt). **Zuschauer** sehen Live-TV, Aufnahmen und Plex, nutzen den TV-Guide und die Fernbedienung und planen Aufnahmen – sie benennen oder löschen keine Aufnahmen, ändern keine Timer anderer Personen und sehen keine Einstellungen über ihre eigenen hinaus. Pro Zuschauer kann ein Admin „Aufnahmen planen“ sowie „Aufnahmen umbenennen und löschen“ erlauben. **Startseite gestalten** legt die Startseite fest, die jeder Zuschauer sieht, bis er seine eigene ändert.

### Mehr als eine Box

Unter Einstellungen → **Boxen** (Admins) fügst du eine weitere Dreambox mit ihrer Adresse und – falls ihre Weboberfläche eines verlangt – ihrem Login hinzu; **Testen** zeigt, ob sie antwortet und der Login stimmt. Ab zwei Boxen erscheint oben in der Leiste eine **Box-Auswahl**, und die Startseite zeigt einen schmalen Streifen mit dem, was die anderen Boxen gerade tun – antippen wechselt. Alles darunter (TV-Guide, Timer, Aufnahmen, Fernbedienung) bezieht sich dann auf die gewählte Box, und die Adresse sagt welche (`/b/wohnzimmer/guide`), Lesezeichen funktionieren also. DreamHub merkt sich die letzte Box pro Gerät.

Eine so hinzugefügte Box wird über ihre Weboberfläche gesteuert: Live-TV, Guide, Timer, Senderlisten, Fernbedienung und das Ansehen ihrer Aufnahmen. Aufnahmen direkt verwalten, Logos, Apps und die Systemwerkzeuge brauchen DreamHub auf der Box selbst. Eine Box, die nicht antwortet, ist in der Auswahl ausgegraut und die Statuszeile sagt es – DreamHub wartet nie auf sie. In den Einstellungen eines Profils lässt sich begrenzen, welche Boxen es nutzen darf (nichts angehakt heißt alle). Eine Box zu entfernen lässt DreamHub sie und alles Gespeicherte vergessen; die Box selbst bleibt unberührt.

**Andere Receiver als eine Dreambox (VU+ und weitere OpenWebif-Boxen).** Alles mit dem OpenWebif-Webinterface lässt sich genauso als Basic-Box hinzufügen – bisher geprüft an einer **VU+ Solo2 mit dem Image VTi 15**. Live-TV, TV-Guide, Timer (mit dem, was jeder Tuner gerade tut), Senderlisten, Fernbedienung, Screenshots und das Ansehen ihrer Aufnahmen funktionieren – eine Aufnahme lässt sich auch auf dem Fernseher dieser Box abspielen, vom Anfang an; DreamHub selbst läuft auf einer Dreambox oder einem Server und erreicht den Receiver übers Netz (für DreamHub selbst haben diese Boxen zu wenig Platz und kein systemd). Dort nicht verfügbar: AutoTimer und SerienRecorder, sofern die Box sie nicht hat, und alles, was DreamHub auf der Box selbst braucht. Receiver im WLAN sind bei großen Senderlisten langsam – wenn möglich per Kabel anbinden. Noch nicht geprüft: eine Box mit eingeschaltetem Web-Login sowie andere Hersteller und Images; sag uns, was du findest.

Eine ausgeschaltete Box lässt sich **von hier einschalten**. Unter Einstellungen → Boxen → Bearbeiten → *Einschalten, wenn die Box aus ist* wählst du **Wake-on-LAN** (MAC-Adresse der Box eintragen; Wake-on-LAN muss in den Netzwerkeinstellungen der Box an sein, und DreamHub im selben Netz) oder **die Aktion einer Erweiterung** (zum Beispiel die Steckdose, an der die Box hängt). Eine Box, die nicht antwortet, zeigt dann einen Knopf **Einschalten** – in der Statuszeile, wenn es die gewählte Box ist, im Streifen der Startseite bei den anderen. Wake-on-LAN geht sofort raus; die Aktion einer Erweiterung wird vorher mit deren eigenem Wortlaut gezeigt, weil die Aktion einer Steckdose meist *umschaltet* – prüfe, dass „ein“ dasteht. Danach schaut DreamHub bis zu drei Minuten lang alle paar Sekunden nach der Box, und sie ist wieder normal, sobald sie antwortet. Jedes Profil, das die Box nutzen darf und die Fernbedienung hat, kann das.

**Eine Aufnahme auf einer anderen Box ansehen.** Mit zwei oder mehr Boxen bietet eine geöffnete Aufnahme **Auf <Box> abspielen** für jede andere Box, die an ist und DreamHub hat (die Box mit DreamHub oder eine gekoppelte). Die Aufnahme startet im Movie-Player dieser Box, mit Vor- und Zurückspulen; DreamHub liest sie von der Box, auf der sie liegt – das darf auch eine Basic-Box sein. Die Box, auf der sie läuft, muss die Adresse von DreamHub erreichen können (dieselbe wie bei Plex – *StreamBase* unter Erweitert in den Einstellungen der Box). Braucht die Berechtigung für die Fernbedienung.

### Empfohlen: Netzwerkzugriff schützen

* **Setze ein DreamHub-Passwort** (Einstellungen → Zugriff → Authentifizierung). Ohne Passwort kann jeder in deinem Heimnetzwerk DreamHub nutzen.
* **Sichere auch das eigene Webinterface der Box.** Standardmäßig erlauben viele Boxen jedem Gerät im Heimnetzwerk die Nutzung von WebControl (Port 80) ohne Passwort, selbst wenn eines gesetzt ist – DreamHub kann das nicht für dich ändern. Setze in den Einstellungen des Webinterfaces der Box **`localauth`** auf *ein* (`config.plugins.Webinterface.localauth=true`). Wenn du das tust, trage die Anmeldedaten der Box unter **Einstellungen → Dienste → WebControl** ein, damit DreamHub sie weiterhin erreichen kann.

### Optionale Schlüssel (API-Keys)

DreamHub funktioniert ohne diese Schlüssel; jeder fügt jedoch Funktionen hinzu. Alle werden unter **Einstellungen** eingegeben, nur auf deiner Box gespeichert und nach dem Speichern nie wieder angezeigt.

| Schlüssel | Kostenlos? | Welchen Mehrwert es bringt |
| --- | --- | --- |
| **TMDB** (themoviedb.org → Einstellungen → API) | ja | Poster, Hintergründe (Backdrops), Bewertungen, Beschreibungen überall; „Cover speichern“; Namensvorschläge; Film-/Bewertungsfilter im TV-Guide |
| **OMDb** ([omdbapi.com/apikey.aspx](https://www.google.com/search?q=https%3A%2F%2Fomdbapi.com%2Fapikey.aspx)) | ja, 1.000 Abfragen am Tag | Rotten Tomatoes, IMDb-Bewertung und Metascore in den Details eines Titels |
| **fanart.tv** (fanart.tv → Profil → API) | ja | Das Logo des Titels über den Details: Details einer Aufnahme, Sendungsdetails im TV-Guide und in Empfehlungen, Serienseite. Benötigt auch TMDB (zum Finden des Titels); fanart.tv hat nicht für jeden Titel ein Logo |
| **OpenAI** | kostenpflichtig pro Nutzung, begrenzt durch ein von dir festgelegtes Tageslimit | KI-Empfehlungen (Empfehlungen) und KI-Namensvorschläge |
| **ntfy-Thema** (Topic) | ja | Benachrichtigungen auf deinem Smartphone – siehe *Benachrichtigungen* unten |

Wenn das **DreamMediaInfo**-Plugin auf der Box installiert ist, nutzt DreamHub zuerst das bereits Bekannte und fragt TMDB nur nach dem Rest. Cover-Bilder, die ein anderes Plugin bereits neben einer Aufnahme gespeichert hat, werden immer bevorzugt.

---

## 3. Was ist wo

### Übersicht (Home)

Ein Dashboard aus Kacheln: **Jetzt auf der Box** (was die Box anzeigt, mit Sender rauf/runter, Stummschaltung und Fernbedienung), anstehende Timer, Konflikte, Speicherplatz, kommende Serienfolgen, Hub-Status. **Bearbeiten** ordnet Kacheln neu an (Pfeile auf jeder Kachel bewegen sie nach oben oder unten; mit der Maus kannst du auch ziehen); **Eigene Kachel** erstellt deine eigene Ansicht (z. B. „Filme von TMDB mit einer Bewertung über 7 heute Abend“).

### Aufnahmen

Jeder Aufnahmeordner auf der Box, Ordner für Ordner.

* **Klicke auf einen Titel** für seine Details: Bild, Bewertungen, Beschreibung und **Auf dem Fernseher abspielen**.
* **Eine Aufnahme umbenennen**: Alle dazugehörigen Dateien werden zusammen umbenannt; Vorschläge kommen von der Aufnahme selbst, TMDB oder KI. Du kannst auch den Titel ändern, den die eigene Liste der Box anzeigt.
* **Mehrere auswählen** (oder **Alle auswählen**) und:
* **Nach Schema umbenennen** – Namen folgen deinem Schema aus *Einstellungen → Aufnahmen* (z. B. `{title} - S{season:02}E{episode:02} - {episodeTitle}`). Wenn ein TMDB-Schlüssel gesetzt ist, werden das fehlende Erscheinungsjahr eines Films und der Episodentitel automatisch von TMDB ergänzt. Aktiviere **In Ordner einsortieren**, um sie zusätzlich nach deinem Ordnerschema in Ordner zu verschieben (z. B. `{title}/Staffel {season:02}`); fehlende Ordner werden erstellt.
* **Cover speichern** – schreibt Poster und Hintergrundbild neben Aufnahmen, die noch keine haben, sodass die eigene Aufnahmeliste des TVs und andere Plugins diese ebenfalls anzeigen (DreamHub zeigt Poster an, ohne sie zu speichern). Bestehende Cover werden niemals überschrieben.
* **In den Papierkorb verschieben** – nichts wird sofort gelöscht; der Tab **Papierkorb** stellt Dateien wieder her oder löscht sie endgültig.


* **Von Hand schneiden** – in den Details einer Aufnahme: nenne die Teile, die herausgeschnitten werden sollen (`m:ss`, von … bis …), sieh dir jede Schnittstelle im Browser an, Vorschau, bestätigen. Dafür ist keine Werbeerkennung nötig; es wird nichts neu codiert (ein Schnitt kann bis zu eine Sekunde danebenliegen), und das Original kommt in den Papierkorb und lässt sich wiederherstellen.
* **Vorzeitig beendet** – eine Aufnahme, die deutlich kürzer ist als die angekündigte Sendung (Neustart, volle Platte, von Hand beendeter Timer), sagt das in der Liste. Eine selbst geschnittene Aufnahme wird nicht markiert.
* **Werbung** – in den Details einer Aufnahme findet **Werbung erkennen** die Werbeunterbrechungen anhand des Senderlogos direkt auf der Box im Hintergrund (es wartet, während die Box aufnimmt). Die Unterbrechungen werden auf einer Zeitleiste angezeigt; tippe auf eine, um von dort im Browser zu schauen. **Werbung schneiden …** hakt die Unterbrechungen ab, bei denen es sich sicher ist – bei unsicheren entscheidest du – zeigt an, was passieren wird, und schneidet sie ohne Neucodierung (Re-Encoding) heraus. Das Original behält alle seine Dateien und wandert in den Papierkorb, sodass es wiederhergestellt werden kann. Für mehrere Aufnahmen gleichzeitig: Wähle sie aus und klicke auf **Werbung erkennen**. Die Liste zeigt „14 Min. Werbung“ neben einer geprüften Aufnahme an.

Dateien werden nur geändert, wenn DreamHub direkt auf der Box läuft, und niemals, während eine Aufnahme noch geschrieben oder abgespielt wird.

### Plex-Mediathek

Die Filme und Serien deines **Plex**-Servers. Verbinde ihn unter **Einstellungen → Dienste → Plex → Bei Plex anmelden**: Öffne plex.tv/link, gib den angezeigten Code ein und wähle dann deinen Server sowie die Adresse aus, die die Box verwenden soll (eine im Heimnetzwerk ist am schnellsten, oder gib eine eigene ein, z. B. einen Reverse Proxy). Die anderen Adressen des Servers werden als **Ausweichoptionen (Fallbacks)** aufbewahrt: Reagiert die verwendete Adresse nicht mehr, wechselt DreamHub zur nächsten, die als derselbe Server antwortet. Du kannst sie unter „Weitere Adressen des Servers“ bearbeiten. Wähle an derselben Stelle aus, welche Mediatheken angezeigt werden sollen.

* Detail- und Serienseiten zeigen das Logo des Titels aus Plex, wenn der Server eines hat.
* **Kacheln oder Liste** – oben rechts umschalten; nach Text oder Gesehen-Status filtern, nach Titel, kürzlich hinzugefügt, Jahr oder Bewertung sortieren.
* **Gesehen** stammt direkt von Plex: Das Markieren eines Films oder einer Episode ändert dies sofort in Plex (mit **Rückgängig**); eine ganze Staffel oder Serie zeigt zuerst jede Episode an.
* **Auf der Box abspielen** spielt die Datei direkt vom Plex-Server mit dem eigenen Player der Box von Anfang an ab – keine Plex-App auf der Box erforderlich – im Movie-Player der Box, sodass du mit der Fernbedienung spulen und pausieren kannst. Ob eine Datei abgespielt werden kann, hängt von ihren Formaten ab, die in den Details angezeigt werden.
* Wenn der Plex-Server ausgeschaltet ist, zeigt die Mediathek weiterhin an, was DreamHub zuletzt gesehen hat („Plex offline · Stand …“); Abspielen und Markieren funktionieren wieder, sobald er antwortet.
* Auf der **Startseite (Home)** kann über **Bearbeiten** die Kachel **Weitersehen (Plex)** aktiviert werden.

### Serien

Deine Serien aus dem **SerienRecorder**. Die Seite einer Serie zeigt geplante Episoden, weitere Ausstrahlungen im TV-Guide – jeweils mit Episodennummer und der Info, ob sie dir **fehlt** – deine Aufnahmen nach Staffeln und (mit TVmaze, standardmäßig aktiv) die vollständige Episodenliste mit dem, was aufgenommen, geplant, fehlend oder noch ausstehend ist. „Fehlend“ wird nur angezeigt, wenn deine Aufnahmen Episodennummern wie `S01E02` tragen. Tägliche Soaps, die der SerienRecorder fortlaufend nummeriert (`S00E8622`), werden anhand dieser Nummer zugeordnet; erst ab deiner ersten Aufnahme zählen vorherige Folgen als fehlend. Eine Ausstrahlung, die auf mehreren Regionalsendern gleichzeitig läuft (z. B. RTL-Fenster), wird nur einmal aufgeführt.

### TV-Guide (EPG)

Vier Ansichten, oben umschaltbar; deine Auswahl wird gespeichert:

* **Raster** – die klassische Zeitleiste, jeweils vier Stunden auf einmal.
* **Jetzt & Danach** – was auf jedem Sender gerade läuft (mit Fortschrittsbalken) und was als Nächstes kommt; **Umschalten** wechselt den Sender am Fernseher. Standard auf Smartphones.
* **Heute Abend** – was zwischen ca. 20:00 und 21:30 Uhr startet, als Bildkarten, bis zu eine Woche im Voraus.
* **Sender** – der gesamte Tag eines Senders als Liste.

**Nur Filme** und **Bewertung 7+** filtern jede Ansicht (benötigt TMDB). Jede Sendung öffnet ihre Details mit **Aufnehmen** und, während sie läuft, **Jetzt umschalten**.

### Live-TV

TV-Sender direkt im Browser – auf dem Smartphone, Tablet oder PC, keine App erforderlich. Wähle einen Sender aus der Liste; **Schauen, was der TV zeigt** neben der Senderliste spielt das ab, was die Box gerade anzeigt. Die Box sendet ihr Signal unverändert und dein Gerät dekodiert es, sodass die Box selbst keine zusätzliche Arbeit hat. **Leertaste** pausiert, **M** schaltet stumm, **F** oder ein Doppelklick wechselt in den Vollbildmodus; bei mehreren Tonspuren (andere Sprache, Dolby) wählst du eine unter dem Bild aus.

Die Box hat einen Tuner: Ein anderer Sender funktioniert nur, solange der Fernseher und etwaige Aufnahmen auf derselben Frequenz liegen. Andernfalls weist das Bild darauf hin – **Schauen, was der TV zeigt** funktioniert immer. Im **TV-Guide** hat eine aktuell laufende Sendung auch die Option **Im Browser schauen**, und die **Startseite (Home)** kann eine **Live-TV**-Kachel mit deinen Sendern anzeigen (unter **Bearbeiten** aktivieren). Unter **Aufnahmen** spielt **Im Browser schauen** eine Aufnahme ab, inklusive einer Leiste zum Springen an beliebige Stellen.

Funktioniert in aktuellen Versionen von Chrome, Edge, Firefox und Safari. HD-Sender nutzen den eigenen Videodekoder deines Geräts; ältere oder sehr kleine Geräte können bei HD ruckeln.

### Radio

Deine Radiosender mit Infos zu dem, was jetzt und danach läuft (sofern der Sender Programminformationen sendet). **Auf der Box abspielen** schaltet die Box auf diesen Sender um – du hörst über den Fernseher oder das angeschlossene Soundsystem; der aktuell laufende Sender wird oben angezeigt. **Im Browser** spielt einen Sender stattdessen direkt auf diesem Gerät ab (wie bei Live-TV muss der Tuner frei oder auf derselben Frequenz sein). Unter dem laufenden Sender steht die Zeile, die der Sender zu seinem Programm schickt (Interpret und Titel oder die Sendung), sofern er eine sendet – sie kann bis zu eine halbe Minute brauchen. Jede Sendung öffnet ihre Details mit **Aufnehmen**; die anstehenden Radio-Aufnahmen sind unten aufgelistet. Sender können auch über die Suche oben gefunden werden.

### Timer

Alle Timer auf der Box, nach Tagen geordnet, mit Markierung wahrscheinlicher Tuner-Konflikte. **Deaktivieren** behält einen Timer in der Liste, ohne aufzunehmen; **Aktivieren** schaltet ihn wieder ein (mit vorheriger Konfliktprüfung). **Löschen** entfernt ihn – das Löschen eines Timers, der gerade aufnimmt, stoppt diese Aufnahme, worauf in der Vorschau hingewiesen wird. **Abgeschlossene aufräumen** entfernt alle abgeschlossenen Einmal-Timer in einer einzigen Vorschau (deaktivierte und wiederkehrende Timer bleiben erhalten). **AutoTimer** nimmt automatisch alles auf, was seinen Regeln entspricht; die Seite listet sie auf und **Zeigen, was aufgenommen würde** gibt eine Vorschau auf die dadurch hinzugefügten Aufnahmen. Jede Zeile hat eine sichtbare Aktion (ein-/ausschalten oder löschen, wenn er nicht geschaltet werden kann); der Rest befindet sich unter **⋯**.

### Empfehlungen

Die KI wählt aus dem Programm eines Bouquets für die nächsten Stunden Empfehlungen aus, optional angepasst an deinen Geschmack (was du aufnimmst und verfolgst – **Was gesendet wird** zeigt die genaue Liste, bevor irgendetwas gesendet wird). Die Empfehlungen bleiben bestehen, bis du neue generierst, auch auf anderen Geräten. Klicke auf eine Empfehlung für ihre Details und **Timer erstellen**.

### Senderliste (Bouquets)

Deine Bouquets, auf dem Smartphone bearbeitbar (Admins). Bei mehr als einer Box gleicht **Von einer anderen Box kopieren** (unter den Bouquets) die Bouquets dieser Box an die einer anderen an: gleichnamige Bouquets werden angeglichen, fehlende angelegt, alles andere bleibt; Sender, die diese Box nicht kennt, werden aufgelistet und ausgelassen. Verschiebe Sender (Schaltflächen oder Ziehen am Computer), setze einen Sender auf eine bestimmte Nummer, benenne um, entferne, füge aus allen der Box bekannten Sendern hinzu, füge Überschriften ein; erstelle, benenne um und lösche Bouquets. **Alle Sender** durchsucht alles, was die Box kennt (neue Sender, Sender in keinem Bouquet, HD, nach Anbieter). **Logos** listet Sender ohne Logo auf sowie Dinge, die einen Blick wert sind (Sender, die nach einem Suchlauf weg sind, doppelt in einem Bouquet, ohne Programmdaten), zeigt die installierten Logo-Pakete und lässt dich das von DreamHub genutzte Paket auswählen – und für einen einzelnen Sender ein abweichendes Logo: aus jedem installierten Paket, ein eigenes Bild oder eines aus der öffentlichen TV-Logos-Sammlung (auch aus der Liste „Ohne Logo“ der Prüfung). Diese Auswahl ändert nur die DreamHub-Bildschirme, niemals den Fernseher, und **Zurück zu Automatisch** macht es rückgängig. Die Liste liest immer die aktuelle Senderliste der Box; sie sucht nicht nach neuen Sendern – führe dafür einen Sendersuchlauf auf der Box durch. Jede Änderung wird gesammelt und als eine Vorschau angezeigt („ZDF HD wechselt von 14 auf 2“); beim Anwenden wird zuerst ein Backup der gesamten Senderliste erstellt, das automatisch wiederhergestellt wird, falls das Ergebnis nicht der Vorschau entspricht. Ältere Backups können wiederhergestellt werden. Benötigt das BouquetEditor-Plugin auf der Box.

### Fernbedienung

Eine Fernbedienung für die Box im Browser mit Live-Bild des TV-Bildschirms und **Nachricht an den TV**, um einen kurzen Text auf dem Bildschirm anzuzeigen. Lässt sich über das Fernbedienungssymbol oben auf jeder Seite öffnen.

### Gerät

**Übersicht** zeigt die Box, ihre Festplatten/Speicher und das **Änderungsprotokoll** jeder über DreamHub vorgenommenen Änderung. **Diagnose** führt den Selbsttest der Box aus. **Sicherung und Wiederherstellung** der eigenen Einstellungen und des Dashboards von DreamHub befinden sich unter **Einstellungen → System** (Admins, die Docker-Apps ausführen, können auch die Konfiguration jeder App in dieses Backup einbeziehen).

**Dienste & Protokolle** (nur Admins) zeigt die **Dienste** der Box – enigma2, DreamHub, Samba und andere – mit ihrem Status; **Neustart** (oder **Start**, wenn einer gestoppt ist) zeigt zuerst an, was dadurch unterbrochen wird, z. B. eine aktuell laufende Aufnahme. Darunter befinden sich Protokolle (Logs) – DreamHub-eigene und die der Box – auf Wunsch live; **Protokolle** neben einem Dienst zeigt nur dessen Zeilen sowie Absturzprotokolle (Crashlogs).

**Erweitert** (nur Admins) ist für Personen, die sich mit der Shell auskennen: eine **Skript-Bibliothek** (Skript hochladen, mit Bestätigung ausführen) und ein **Terminal** – standardmäßig deaktiviert, erfordert das Aktivieren und erneute Eingeben des eigenen Passwortschutzes von DreamHub zum Einschalten, mit einer dauerhaften Warnung, solange es aktiv ist.

### Apps

Die Seite hat drei Tabs in fester Reihenfolge – **Pakete**, **Erweiterungen**, **Docker** –, die jeweils mit einem Satz öffnen, der erklärt, was es ist. DreamHub merkt sich den zuletzt angesehenen Tab (pro Browser).

**Pakete** ist die eigene Software der Box (DreamOS): TV-Plugins, Skins und Tools. Suche oben, dann die Filter **Installiert**, **Verfügbar** und **Updates** sowie Kategorie-Chips (Erweiterungen, Skins, System-Plugins, Treiber, Senderlogos, Kernel-Module, System). Wenn Updates ausstehen, zeigt eine Leiste „3 Updates verfügbar“ mit **Alle aktualisieren** – eine Vorschau listet jedes Paket auf, das `apt` anfassen würde, und eine Bestätigung wendet sie an. Jede Zeile hat eine Aktion (Installieren, Aktualisieren oder Entfernen) und ein **⋯**-Menü mit den Details (Version, Herkunft der Beschreibung). Jede Zeile erklärt in einem Satz in deiner Sprache, was das Paket ist; wo DreamHub keinen eigenen Text hat, zeigt es die eigene Beschreibung des Pakets an, und für ein Paket ohne beides erscheint ein optionaler Eintrag **KI fragen** in **⋯** nur, wenn ein KI-Schlüssel eingerichtet ist – zwingend erforderlich ist er für nichts. Ein Paket, das von einer DreamHub-Funktion verwendet wird, zeigt „Benötigt von Aufnahmen“ (und so weiter), und das Entfernen erfordert eine vorherige Bestätigung. DreamHubs eigene Beschreibungsliste ist noch kurz; sie wächst, während der Paket-Feed der Box gelesen wird. Das Zahnrad öffnet **Paketquellen (Feeds)** – schalte sie ein/aus oder füge eine hinzu, jeweils mit Vorschau; beim Hinzufügen wird gewarnt, dass diese Feeds unsigniert sind, füge also nur vertrauenswürdige hinzu.

**Erweiterungen** sind kleine Add-ons für DreamHub selbst – zum Beispiel eines, das eine smarte Steckdose schaltet. Jede wird als Karte mit eigenem Status und Aktionen angezeigt (niemals als eigene Webseite); eine Erweiterung, die Eingaben erfordert (z. B. einen Geräteschlüssel), verfügt über eine Schaltfläche **Einstellungen**.

**Docker** – optional: zusätzliche Apps in Containern, für den normalen Betrieb nicht erforderlich. **Apps** sind Programme, die in einem eigenen Docker-Container auf der Box laufen, getrennt von der TV-Software. Wenn Docker noch nicht eingerichtet ist, weist ein Hinweis darauf hin und verweist auf die **Docker-Einstellungen** (das Zahnrad oben rechts im Tab), wo **Docker einrichten** anzeigt, was getan wird, bevor es ausgeführt wird. Eine kurze, handverlesene Liste (AdGuard Home, Pi-hole, Nextcloud, Vaultwarden, Syncthing, Home Assistant, qBittorrent) zeigt, ob die jeweilige App auf dem Prozessor, dem Arbeitsspeicher und Port deiner Box läuft, sowie eine Anmerkung für diese Box; eine Suche nach allem anderen auf Docker Hub ist ausgeblendet (ein Ergebnis kann auf Kompatibilität geprüft oder als eigener Container hinzugefügt werden). **Installieren** öffnet ein kleines Formular: den **Web-Port** (nur änderbar, wo das Image der App es zulässt – andernfalls wird darauf hingewiesen), die **Festplatte/Speicher** zum Speichern der App (nur echte Datenträger, niemals der eigene Flash-Speicher der Box) und jedes von der App benötigte Feld; **Erweiterte Optionen anzeigen** fügt jedes weitere Feld hinzu sowie freie Zeilen für **Umgebungsvariablen**, **Ports** und **Ordner** sowie ein Arbeitsspeicherlimit. Ein geheimes Feld (ein Passwort) wird während der Eingabe verborgen und nie wieder angezeigt. Anschließend wird genau angezeigt, was erstellt wird – Images, Ports, die Ordner auf der Box, die Variablen –, geprüft gegen freien Arbeitsspeicher, Speicherplatz und Ports, bevor irgendetwas läuft. Der Tab bietet zwei Ansichten: **Meine Container** (was auf der Box ist) und **App-Katalog** (was du hinzufügen kannst). In **Meine Container** hat jede Zeile das Bild der App, den Namen, einen Statuspunkt, ihren Port, einen **Autostart**-Schalter (startet sie immer, wenn die Box startet; wird sofort gespeichert) und eine Hauptschaltfläche – **Öffnen**, während sie läuft, ansonsten **Starten**. Hinter **⋯**: **Stopp**, **Protokolle**, eine **Konsole** (eine interaktive Shell innerhalb der App – dieselbe Handhabung wie beim Terminal: standardmäßig aus, Passwort-Neueingabe, dauerhafte Warnung), **Bearbeiten** (dasselbe Formular wie bei der Installation, vorausgefüllt; es zeigt an, was sich ändert und erstellt die App neu – ihre Daten bleiben erhalten), **Sichern**, **Aktualisieren** und **Entfernen** (ihre Einstellungen bleiben erhalten). Ein Badge „Update verfügbar“ und eine Benachrichtigung erscheinen, wenn eine neuere Version existiert – DreamHub aktualisiert eine App niemals von selbst. Einige Apps bestehen aus mehr als einem Container – Nextcloud bringt eine eigene Datenbank mit, die zusammen mit ihr verwaltet und darunter mit eigenen Protokollen aufgelistet wird. Apps teilen sich das Netzwerk der Box, daher ist jeder Port der Port der Box; auf der Referenzbox ist Port 53 bereits durch den eigenen DNS-Resolver der Box belegt, sodass AdGuard Home und Pi-hole dort nicht installiert werden können. Unter **Einstellungen → System → Sicherung** ist jeder App-Ordner mit seiner Größe aufgelistet: eine Datenbank ist standardmäßig enthalten, große Dateien (die von Nextcloud, Downloads) nur, wenn sie angehakt sind.

**Container hinzufügen** (über der Liste, nur Admins) ist für alles, was nicht in der Liste steht: Füge eine **Compose-Datei** (`docker-compose.yml`) ein, gib einen **Image-Namen** ein oder füge ein **Unraid-Template** ein (die XML; DreamHub bündelt keine Templates und lädt nichts herunter – es liest nur Ports, Ordner und Variablen). Du erhältst dasselbe Formular wie für die Liste, aufgebaut aus der Datei: Jedes `${NAME:-default}` darin wird zu einem Feld. Was verstanden wird: Dienste mit Image, Ports, Volumes, Environment, Command, einem Arbeitsspeicherlimit, Restart und `depends_on`; alles andere (`build`, `networks`, `secrets`, …) wird mit Begründung abgelehnt. Apps nutzen das eigene Netzwerk der Box, daher kann ein Port nicht auf einen anderen gemappt werden – die App lauscht auf ihrem eigenen Port. Ein Ordner, der als `.` oder `./name` geschrieben ist, wird im Speicher der App aufbewahrt; jeder andere Ordner muss unterhalb von `/media` liegen. **Du bist selbst verantwortlich für das, was du ausführen lässt**: DreamHub verweigert einem Container jeglichen Systemordner der Box (`/`, `/etc`, `/usr`, `/var`, `/boot`, `/proc`, `/sys`, `/dev`), die eigenen Daten von DreamHub, den Docker-Socket und die Daten anderer Apps – auch über Links – und fordert eine zusätzliche Bestätigung an, bevor es einem Container den Privileged-Modus, zusätzliche Capabilities, Geräte oder die Prozessansicht der Box gewährt; es kann die Software selbst nicht beurteilen. Von dir hinzugefügte Container werden über **⋯** wie die anderen bearbeitet und aktualisiert, außer solche, die hinzugefügt wurden, bevor die Bearbeitung existierte (diese entfernen und erneut hinzufügen).

**Updates.** Wenn etwas aktualisiert werden kann, zeigt eine Box oben auf der Seite an, wie viele Pakete und Container, mit **Alle aktualisieren**. Die Vorschau listet alles in der Reihenfolge auf, in der es ausgeführt wird: die Container nacheinander, dann die Pakete. Container kommen zuerst, da ein Update, das nicht läuft, von selbst zur vorherigen Version zurückkehrt, sodass sich bei einem Fehlschlag nichts anderes ändert; die Pakete folgen nur, wenn jeder Container in Ordnung ist. Wenn ein Schritt fehlschlägt, wird der Rest nicht gestartet, und das Ergebnis gibt an, was getan wurde, was fehlgeschlagen ist und was nicht gestartet wurde. DreamHub selbst ist kein Teil davon (ein Hinweis weist darauf hin); aktualisiere es mit den eigenen Tools der Box. Während eine Aufnahme läuft oder eine innerhalb von 15 Minuten startet, wird nichts aktualisiert: Die Box meldet „Eine Aufnahme läuft … Updates sind ab 21:15 Uhr wieder möglich.“ **Automatische Updates** sind standardmäßig ausgeschaltet: ein Schalter für Container in den Docker-Einstellungen, einer für Pakete in den Paket-Einstellungen (nur Pakete, die enigma2 nicht neustarten); „Nur aktualisieren zwischen“ beschränkt sie auf eine Tageszeit (das manuelle Drücken von Aktualisieren wird nur durch eine Aufnahme zurückgehalten). Ihre Ergebnisse erscheinen in den Benachrichtigungen. Im **⋯**-Menü eines Containers aktualisiert **Aktualisieren** diesen Container, und **Zurückrollen** (angezeigt, solange die vorherige Version aufbewahrt wird) kehrt zurück; die Docker-Einstellungen listen die aufbewahrten Versionen auf und können sie entfernen. Im Menü eines Pakets erscheint **Downgrade auf …** nur, wenn der Feed noch eine ältere Version bereitstellt. Wenn eine App immer wieder stoppt, verlinkt eine Benachrichtigung zu ihren Protokollen.

**Sicherung und Wiederherstellen von Apps.** *Einstellungen → System → Sicherung & Wiederherstellung* kann deine Apps einbeziehen: Hake die Apps (und welche ihrer Ordner) an und lade die Sicherung herunter, oder wähle **Auf der Box speichern**, um sie auf der Aufnahmefestplatte aufzubewahren, was sich für große App-Daten eignet. Der App-Teil enthält auch die Passwörter, die DreamHub für die Apps erstellt hat, bewahre die Datei also sicher auf. Stelle die Datei auf einer neuen Box wieder her; die Apps erscheinen dann unter *Meine Container* als **Apps, die du wiederherstellen kannst** mit *Erneut installieren*. Du siehst die übliche Vorschau und bestätigst sie. Eine zuvor entfernte App wird auf dieselbe Weise angeboten.

**Docker-Einstellungen** (das Zahnrad) zeigen an, ob Docker läuft, und **Docker einrichten**, wenn es nicht läuft. Sobald es läuft, enthalten sie außerdem: wo **neue** Apps ihre Daten aufbewahren (die Datenträger mit ihrem freien Speicherplatz; bereits installierte Apps bleiben dort, wo sie sind), ob neue Apps mit der Box starten, das **Arbeitsspeicherlimit pro App** (automatisch oder deine eigene Zahl für Container, die keines angeben), wie oft DreamHub **nach neueren App-Versionen sucht** und ob es dich in den Benachrichtigungen informiert (nichts wird jemals von selbst aktualisiert), sowie **Ungenutzte Images aufräumen** – alte Images bleiben auf dem Datenträger, wenn eine App entfernt oder aktualisiert wird; es zeigt an, wie viel Speicherplatz das ist, listet sie dann auf und fragt nach, bevor gelöscht wird. Jede Einstellung zeigt **Zurücksetzen** nur an, wenn sie vom Standard abweicht. Am Ende: **Docker deinstallieren**. Die Deinstallation ist eine Vorschau wie jede Änderung: Sie stoppt und entfernt die von DreamHub erstellten App-Container, schaltet Docker und den Docker-Helper von DreamHub aus, stoppt das Laden des Overlay-Kernel-Moduls beim Booten und löscht die Docker-Programmdateien. Es stellt eine Frage – **Auch App-Daten löschen?** – und behält diese standardmäßig bei, sodass die Apps nach dem erneuten Einrichten von Docker wiederkommen. Aufnahmen und die TV-Software werden niemals angerührt.

---

## 4. DreamHub als App

DreamHub kann auf dem Startbildschirm deines Smartphones oder dem Desktop deines Computers abgelegt werden und sich in einem eigenen Fenster öffnen, ohne die Adressleiste des Browsers. **Einstellungen → Allgemein → DreamHub als App** zeigt dir, was in dem von dir verwendeten Browser funktioniert, inklusive einer **Installieren**-Schaltfläche, sofern der Browser eine anbietet.

* **iPhone / iPad:** Tippe in Safari auf **Teilen → Zum Home-Bildschirm**. Dies funktioniert über einfaches `http://`, ohne weitere Einrichtung.
* **Android, Windows, Mac, Linux (Chrome, Edge):** Browser bieten die Installation nur über **HTTPS** an, und DreamHub auf der Box ist einfaches `http://` (für eine LAN-Adresse kann kein Zertifikat existieren, dem ein Smartphone vertrauen würde). Solange es nicht hinter HTTPS liegt, nutze im Browser-Menü **Zum Startbildschirm hinzufügen**: Das gibt dir ein Icon, nur ohne das eigenständige App-Fenster.

**DreamHub hinter HTTPS bereitstellen** (optional) erfordert ein zweites Gerät, das dauerhaft eingeschaltet ist, sowie einen Namen mit einem echten Zertifikat – zum Beispiel:

* **Tailscale:** Auf jedem dauerhaft laufenden Gerät in deinem Netzwerk liefert `tailscale serve --bg [http://192.168.1.43:8765](http://192.168.1.43:8765)` eine Adresse wie `https://<gerätename>.<tailnet>.ts.net`, erreichbar von deinen Geräten zu Hause und unterwegs.
* **Ein Reverse Proxy**, den du bereits betreibst (Caddy, Nginx Proxy Manager, Traefik auf einem NAS), mit einer Domain, die darauf verweist und an `http://<box-IP>:8765` weiterleitet.

Trage diesen Hostnamen in jedem Fall unter **Einstellungen → Zugriff → Netzwerk → Zusätzlich erlaubte Hostnamen** ein, da DreamHub an ihn gerichtete Anfragen andernfalls verweigert (Schutz vor DNS-Rebinding).

Über HTTPS dekodiert der Browser HD-Sender im Live-TV zudem mit der eigenen Hardware des Geräts (über einfaches `http://` verweigert er dies), sodass HD auf Smartphones flüssiger läuft. Bestätigt mit Nginx Proxy Manager auf einem Unraid-Server (08.10.2026). Wenn das Gerät, auf dem der Proxy läuft, ausgeschaltet ist, ist auch die HTTPS-Adresse nicht erreichbar – die eigene Adresse der Box (`http://<box-IP>:8765`) funktioniert weiterhin.

**Behalte es in deinem Heimnetzwerk.** Nutze für den Zugriff von unterwegs Tailscale (oder ein anderes VPN), anstatt den Proxy für das Internet zu öffnen. Wenn DreamHub von außen erreichbar ist, schalte zuerst die Anmeldung ein (**Einstellungen → Zugriff**): Ohne diese kann jeder, der die Adresse erreicht, die Box steuern.

Wenn die Box ausgeschaltet oder nicht erreichbar ist, öffnet sich die App trotzdem, weist darauf hin und verbindet sich von selbst, sobald die Box wieder antwortet. Nichts von der Box wird auf dem Smartphone gespeichert: Timer, Aufnahmen und der TV-Guide sind immer live.

---

## 5. Benachrichtigungen auf deinem Smartphone

*Im Browser:* Die Glocke oben bietet **Benachrichtigungen auf diesem Gerät anzeigen**. Der Browser zeigt dann eine Meldung an, wenn DreamHub in einem Hintergrund-Tab oder als installierte App geöffnet ist – dies erfordert HTTPS (oder `localhost`) und deine Erlaubnis. Bei geschlossenem DreamHub nutze ntfy unten.

DreamHub kann eine kurze Nachricht an dein Smartphone senden, wenn etwas passiert, über die kostenlose App **ntfy** (iPhone und Android; kein Konto erforderlich):

1. Installiere **ntfy** auf deinem Smartphone.
2. In DreamHub unter **Einstellungen → Benachrichtigungen**: Gib ein **Thema (Topic)** ein – einen schwer zu erratenden Namen; die Seite schlägt einen vor – und speichere.
3. Abonniere in der ntfy-App dasselbe Thema (Server `ntfy.sh`, sofern du keinen eigenen betreibst) und drücke dann in DreamHub auf **Test senden**.

Wähle aus, welche Ereignisse eine Nachricht senden: Eine Aufnahme ist beendet (ein), eine Aufnahme hat gestartet (aus), **eine fehlende Episode einer deiner Serien läuft in den nächsten 24 Stunden** (ein), ein möglicher Timer-Konflikt (ein), der Datenträger ist fast voll (ein), **ein Dienst auf der Box fällt aus oder kehrt zurück** (ein – Samba, die Softcam, enigma2 selbst; DreamHub prüft jede Minute und meldet nur Dienste, die dauerhaft ausfallen). Die Glocke oben in DreamHub zeigt all diese Ereignisse ebenfalls an, mit oder ohne Push.

Datenschutz: Der Text einer Nachricht (Titel einer Aufnahme oder Serie, Sendername) läuft über den ntfy-Server. Auf dem öffentlichen `ntfy.sh` könnte jeder, der dein Thema kennt, diesen mitlesen – deshalb sollte er schwer zu erraten sein. Für die vollständige Kontrolle betreibe deinen eigenen ntfy-Server und gib dessen Adresse ein.

---

## 6. Wenn etwas nicht funktioniert

* **Nicht sicher, was auf deiner Box funktioniert?** **Gerät → Diagnose → Selbsttest ausführen** fragt die Box nach allem ab, was DreamHub benötigt, ohne etwas zu ändern, und markiert jede Antwort: in Ordnung, einen Blick wert, fehlgeschlagen oder nicht zutreffend. **Antworten der Box herunterladen** liefert eine ZIP-Datei, die beim Melden eines Problems hilft.
* **DreamHub startet nicht und im Log steht „address already in use“:** Ein anderes Programm auf der Box nutzt den Port von DreamHub (**8765**). Wähle im TV-Plugin einen freien (**DreamHub → Einstellungen → Backend-Port**; nach deiner Bestätigung startet DreamHub neu) oder im Terminal: `echo 'DREAMHUB_LISTEN_ADDR=:8800' > /etc/default/dreamhub && systemctl restart dreamhub` – danach `http://<Box>:8800` öffnen.
* **DreamHub öffnet sich nicht:** Überprüfe, ob der Dienst läuft – im WebAdmin-Terminal mit `systemctl status dreamhub`. Sein Protokoll: `journalctl -u dreamhub -n 100`.
* **Ein Hub zeigt „Nicht verfügbar“ oder „Eingeschränkt“:** Der Punkt in der Seitenleiste und die Glocke erklären den Grund und die Lösung (fehlender Schlüssel, nicht installiertes Plugin, keine Antwort der Box). **Gerät → Diagnose** listet alle auf.
* **„WebControl konnte nicht erreicht werden“:** Die Benutzeroberfläche der Box startet neu oder ist beschäftigt (nach einem Update benötigt enigma2 einen Moment) oder verlangt jetzt eine Anmeldung – gib diese unter **Einstellungen → Dienste → WebControl** ein.
* **Datei-Aktionen fehlen unter Aufnahmen:** Sie erfordern, dass DreamHub direkt auf der Box läuft; die Seite weist darauf hin, wenn sie nicht verfügbar sind.
* **Keine Bilder:** Füge einen TMDB-Schlüssel hinzu und stelle sicher, dass *Bilder, Bewertungen und Übersichten laden* aktiviert ist (Einstellungen → Dienste → TMDB).
* **Mehr Details für einen Fehlerbericht:** **Gerät → Dienste & Protokolle → Protokolle**, stelle das Level für eine Weile auf *Debug* (es schaltet sich von selbst wieder zurück).

Probleme und Ideen: Der Issue-Tracker des Repositorys auf GitHub.
