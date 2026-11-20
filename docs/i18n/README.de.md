# launchbay

> Jedes Lesezeichen, das du je vergessen hast, auf einer Seite — selbst gehostet, offline, ohne Registrierung.

[![License: MIT](https://img.shields.io/badge/License-MIT-1f6feb)](../../LICENSE) [![365 Open Source Plan #042](https://img.shields.io/badge/365%20Open%20Source%20Plan-%23042-1f6feb)](https://github.com/rockbenben/365opensource) [![ci](https://img.shields.io/github/actions/workflow/status/rockbenben/launchbay/ci.yml?label=ci&branch=main)](https://github.com/rockbenben/launchbay/actions/workflows/ci.yml)

[▶ Schnellstart](#schnellstart) · [⬇ Releases](https://github.com/rockbenben/launchbay/releases)

[English](../../README.md) · [简体中文](../../README.zh.md) · [繁體中文](README.zh-Hant.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [Español](README.es.md) · [Português](README.pt.md) · [Français](README.fr.md) · **Deutsch** · [Italiano](README.it.md) · [Русский](README.ru.md) · [العربية](README.ar.md) · [हिन्दी](README.hi.md) · [বাংলা](README.bn.md) · [ไทย](README.th.md) · [Türkçe](README.tr.md) · [Tiếng Việt](README.vi.md) · [Bahasa Indonesia](README.id.md)

![launchbay-Profilseite: Suchleiste oben, Gruppen in der Seitenleiste und Kachelkarten](../../docs/home-1280.en.webp)

Bringe die Adressen, die du dir nie merken kannst, auf eine einzige Seite: das NAS-Admin-Panel, die Router-Oberfläche, das Firmen-Intranet, das Monitoring-Dashboard. Mach sie zu deiner Browser-Startseite — **ein Klick, kein Warten auf Ladekreise.**

Es läuft auf deiner eigenen Maschine — einem Heim-NAS, einem Raspberry Pi, einem Software-Router oder einem Server im Firmennetz — sodass deine Lesezeichen niemals fremde Server berühren.

## Warum launchbay

Die meisten Lesezeichen-Tools unterscheiden sich in vier Punkten: auf wessen Server die Daten liegen, wie ein Haushalt sie teilt, ob es offline funktioniert und ob man sie durch Ziehen anordnen kann.

| Vergleichskriterium | Eigene Lesezeichen des Browsers | Cloud-Lesezeichendienste | launchbay |
|---|---|---|---|
| Wo die Daten liegen | Dein Browserprofil, folgt deinem Konto | Server des Anbieters | **Deine eigene Maschine** |
| Pro Person getrennt | Über Browserprofile | Pro Konto | **Jeder ein eigenes, serverseitig getrennt** |
| Offline | Nur lokal | Funktioniert nicht | **Cache + Offline-Warteschlange** |
| Organisieren per Drag & Drop | Ordnerbaum, keine gestaltete Seite | Meist nicht | **Ablegen zum Speichern, Ziehen zum Sortieren** |
| Wer sonst noch mitlesen kann | Dein Browser-Konto | Der Anbieter | **Niemand über die App** |

Kein Konto, keine Drittanbieter-Skripte, keine Telemetrie. Die einzigen externen Anfragen dienen dem Abrufen der hinzugefügten Seite (für den Titel) und deren Favicon; ein fehlgeschlagener Favicon-Abruf wird für 24 Stunden pausiert.

**Wenn du launchbay alleine nutzt, reicht die Lesezeichenseite des Browsers meist aus.** launchbay bietet dafür drei entscheidende Vorteile:

- **Öffnen ohne Wartezeit.** Die Seite wird serverseitig gerendert: Lesezeichen und Icons sind bereits im HTML enthalten, bevor ein Skript ausgeführt wird. Das Ziel ist ein LAN-First-Paint LCP < 300 ms; ein Kaltstart auf einem echten NAS wurde noch nicht gemessen (siehe [Grenzen](#grenzen)).
- **Jeder ein eigenes Profil, und niemand sonst sieht es.** Jedes Profil kann eine eigene 4–6-stellige PIN haben. Der Admin kann Profile anlegen, löschen und PINs zurücksetzen, aber jeder Inhalts-Endpunkt verweigert den Admin-Schlüssel mit einem 403 — es gibt **keine Route** zu deinen Lesezeichen. Das ist keine Kryptografie gegen den Admin oder das LAN (jeder im LAN kann Cookies manipulieren); der Server hält die Profile sauber voneinander getrennt.
- **Fehler lassen sich rückgängig machen.** Vor jeder Massenänderung wird automatisch ein Schnappschuss erstellt. „Zurücksetzen“ zeigt an, wie viele Lesezeichen und welche Gruppen jede ältere Version enthielt, und speichert die aktuelle Version vor dem Wiederherstellen — ein versehentliches Zurücksetzen kann also selbst wieder rückgängig gemacht werden. Der Verlauf wird fortlaufend geführt und nie gelöscht: Jede Zeile dokumentiert, wer wann über welchen Weg (Webseite, KI, Erweiterung, CLI) welchen Eintrag geändert hat — niemals den Inhalt selbst.

## Was abgedeckt ist

| Aspekt | Ob es für dich passt |
|---|---|
| Wie du es ausführst | `docker compose up -d` oder die statische Binärdatei (amd64 / arm64) |
| Wo es läuft | Ein Heim-NAS, Raspberry Pi, Software-Router oder ein Rechner im Firmen-Intranet |
| Wer es nutzt | Eine Einzelperson, eine Familie oder ein kleines Team — je ein Profil pro Person; der Admin verwaltet Konten, keine Inhalte |
| Browser | Jeder moderne Browser; bei deaktiviertem JavaScript bleibt jedes Lesezeichen sichtbar und anklickbar |
| Was du installierst | Nur Docker: keine Drittanbieter-Bibliotheken, keine Datenbank, kein Node zur Build-Zeit |
| Wo die Daten liegen | Ein gewöhnliches Verzeichnis, `data/`; ein Backup ist einfaches Kopieren |
| Oberflächensprache | 4 Oberflächensprachen (vereinfachtes Chinesisch, traditionelles Chinesisch, Englisch, Arabisch RTL); Standard beim Setup wählen, danach stellt jedes Profil seine eigene ein |

## Was es kann

**Speichern und aufräumen**

- **Einfach ablegen und gespeichert**: Ziehe einen Link aus der Lesezeichenleiste, der Adresszeile oder einem anderen Tab auf eine Gruppe. Kein Dialogfenster — das Ablegen genügt.
- **Ziehen zum Sortieren**: Innerhalb einer Gruppe, gruppenübergreifend oder auf einen Gruppentitel zum Anhängen. Gedrückthalten oder ein Tastenkürzel startet die Mehrfachauswahl, um Tags zuzuweisen, zu verschieben oder in einem Schritt zu löschen.
- **Von jeder Seite aus speichern**: Ziehe das Bookmarklet in deine Lesezeichenleiste; ein Klick holt die Adresse, ein weiterer Tastendruck speichert sie.
- **Aus deinem Browser importieren**: Lade die von Chrome / Edge / Firefox exportierte Lesezeichendatei hoch und erhalte einen Bericht pro Eintrag — was übernommen wurde, was nicht und warum. Die Weboberfläche kann direkt importieren oder zuerst den Bericht anzeigen; die CLI schreibt im selben Durchlauf, sofern du nicht `--dry-run` übergibst.
- **Ein ganzes Profil mitnehmen**: Eine Datei enthält die Lesezeichen **und** das Aussehen der Seite (Design, Layout, Ansicht, zweite Zeile, Seitenname, Sprache, Adressmodus). PIN, Profilidentität und Hintergrundbild sind bewusst ausgenommen.

**Finden**

- **Gruppen und Suche**: Die obere Leiste durchsucht Namen, Adressen, Notizen und Tags; drücke Enter und der Server antwortet — ganz ohne JavaScript. Mit aktiviertem JavaScript drücke `/` oder nutze die Palette `⌘/Ctrl K`.
- **Gruppen größer dargestellt**: Hebe deine meistgenutzten Gruppen in einem Bereich mit großen Kacheln ganz oben hervor.
- **10.000+ Icons, offline**: Tabler und Simple Icons sind gzippt in der Binärdatei enthalten; das Favicon einer Website wird einmal abgerufen und danach aus dem lokalen Cache ausgeliefert.

**Zu Hause und unterwegs**

- **Zwei Adressen pro Lesezeichen**: Lege eine primäre und eine optionale WAN-Adresse fest. „Auto“ folgt der Adresse, über die du launchbay geöffnet hast; Einstellungen → Lesezeichenadressen überschreibt dies für den aktuellen Browser. Suche und Kopieren nutzen die aktive Adresse, und Lesezeichen ohne WAN-Adresse behalten die primäre bei. Hinter einem Reverse Proxy den ursprünglichen `Host`-Header beibehalten.

**Die Seite selbst**

- **Ein Einstellungs-Panel**: Obere Leiste → Einstellungen. Hier findest du das Aussehen der Seite, die Sprache, die Gruppenliste, Backup und Verlauf sowie **Meine PIN** — sperre oder entsperre dein eigenes Profil ohne den Admin.
- **Funktioniert offline**: Die letzte Seite wird gecacht, und im Offline-Zustand — oder während der Server aus ist — hinzugefügte Lesezeichen werden eingereiht und automatisch übertragen, sobald der Browser wieder verbunden ist; klicke auf **Jetzt senden**, falls der Server selbst offline war.
- **Skripte und KI haben eigene Schlüssel**: Eine REST-API plus profilgebundene Tokens für die Browser-Erweiterung, die CLI und Automatisierungen. Ein Token, das über sein Profil hinausgreift, erhält einen 403.

## Schnellstart

```bash
git clone https://github.com/rockbenben/launchbay
cd launchbay
docker compose up -d
```

Keine `.env`-Datei, keine Go-Toolchain — das Image wird einfach heruntergeladen.

Öffne **http://<LAN-Adresse dieses Rechners>:7600** von einem beliebigen Gerät im lokalen Netzwerk — **nicht `localhost`**, was „dieses Gerät“ bedeutet und das Smartphone aufrufen würde, auf dem du es eingetippt hast. Wenn du direkt am Hostrechner sitzt, funktioniert `http://localhost:7600`.

Das Image trägt das Tag `:latest`; das erste `up` lädt das neueste Release herunter. Ein Rechner, auf dem der Container bereits läuft, behält sein lokales Image — führe zuerst `docker compose pull` aus, um zu aktualisieren.

> **Ändere zuerst den Standard-Admin-Schlüssel.** compose liefert `launchbay123` mit, was öffentlich bekannt ist. Jeder in deinem LAN, der ihn kennt, kann Profile erstellen, löschen und PINs zurücksetzen (Lesezeichen lesen kann er dennoch nicht). Trage deinen eigenen Schlüssel in `LAUNCHBAY_ADMIN_TOKEN` ein und starte `docker compose up -d` erneut:
>
> ```yaml
> LAUNCHBAY_ADMIN_TOKEN: "dein-eigener-admin-schluessel"
> ```

Beim ersten Aufruf gelangst du auf die einmalige Einrichtungsseite: Benenne das erste Profil und lege eine 4–6-stellige PIN fest. Es gibt keine Standardzugangsdaten, und die Einrichtungsseite schließt sich endgültig, sobald mindestens ein Profil existiert.

**„Keine PIN“ ist ein eigenes Häkchen, kein leeres Feld.** Ein leeres PIN-Feld wird abgelehnt; das Setzen des Häkchens bei **Keine PIN** erstellt ein Profil, das für jeden im LAN offen ist. Das ist Absicht: Der teure Fehler wäre, drei Ziffern einzutippen und versehentlich Enter zu drücken, wodurch die privaten Lesezeichen unbemerkt für das gesamte Haus freigegeben würden. Ein Profil ohne PIN zeigt auf der Auswahlliste kein PIN-Eingabefeld. Du kannst später unter **Meine PIN** jederzeit eine PIN hinzufügen oder entfernen; das Entfernen erfordert die alte PIN nur, solange eine aktiv gesetzt ist.

Das erste Profil ist **kein Administrator**. Die Maschine wird über `LAUNCHBAY_ADMIN_TOKEN` verwaltet: Öffne `/admin`, gib den Schlüssel einmal ein, und du erhältst ein Cookie, das ausschließlich der Admin-Identität gehört (30 Tage gültig).

**Weitere Personen müssen nicht auf den Admin warten.** Die Auswahlliste bietet den einklappbaren Bereich **„Neues für dich“**: Öffne ihn, gib Name und PIN ein, und das Profil existiert (`POST /new`; die Maschinenroute lautet `POST /api/v1/profiles/new`). Es sind keine Zugangsdaten erforderlich, sodass jedes Gerät im LAN ein Profil erstellen kann; die Kachel erscheint, und der Admin kann sie jederzeit löschen. Dadurch entsteht ein **leeres** Profil mit eigener PIN, das keinerlei Zugriff auf die Lesezeichen anderer gewährt. Namen können bis zu 60 Zeichen lang sein; wird ein bereits vergebener Name wiederverwendet, sind die beiden Kacheln optisch nicht zu unterscheiden.

**Das Löschen eines Profils ist die einzige unwiderrufliche Aktion** und erfordert daher erneut den Maschinenschlüssel: Die Bestätigungsseite listet auf, wie viele Lesezeichen und Schnappschüsse verloren gehen, und du musst `LAUNCHBAY_ADMIN_TOKEN` noch einmal eingeben. Die API-Route (`DELETE /api/v1/profiles`) akzeptiert ausschließlich diesen Schlüssel; ein reines Admin-Cookie führt zu 403. Schnappschüsse und private Hintergründe werden zusammen mit dem Profil gelöscht, sodass auch ein Zurücksetzen das Profil nicht retten kann.

Auf derselben Seite befindet sich das Auswahlfeld **Standardsprache**, das du bei der Einrichtung festlegst. Es bestimmt den **systemweiten Standard**; danach kann jeder in seinen eigenen Einstellungen eine eigene Sprache wählen. Änderungen des Admins wirken sich nur auf Profile aus, die keine eigene Sprache gewählt haben.

### Admin-Schlüssel und Tokens

| Variable | Erforderlich? | Was sie abdeckt |
|---|---|---|
| `LAUNCHBAY_ADMIN_TOKEN` | **Erforderlich**, ein einfacher Schlüssel | Nur Kontenverwaltung: Profile erstellen/löschen, beliebige PINs setzen, aktivieren/deaktivieren, profilübergreifender Status und Verlauf. Jeder Inhalts-Endpunkt (`/api/v1/data`, `/batch`, `/export`, …) antwortet mit 403 — Lesezeichen können nicht gelesen werden |
| `LAUNCHBAY_WRITE_TOKENS` | Optional, standardmäßig leer | Profilgebundene Schreib-Tokens im Format `name:secret:profile`. Nur die Browser-Erweiterung, die CLI (`revert` / `import-bookmarks` / `backup` / `restore` / `profile fix-names`) sowie Skripte und KI benötigen eines |

> **Warum standardmäßig leer?** Die Lesezeichenseite, der Admin-Bereich, Zurücksetzen und Importieren stützen sich auf das Sitzungscookie; es wird kein Token benötigt. Die drei Werkzeuge, die ein Token brauchen, werden ganz bewusst eingerichtet. Würde standardmäßig ein Token mitgeliefert, besäße jede Maschine ab Werk einen Schlüssel, der das Profil `dad` beschreiben könnte (`dad` ist die häufigste Profil-ID).

Ein Profil besitzt kein Attribut „ist Admin“. Die Admin-Seite zeigt Aktionen auf Kontoebene sowie für jedes Profil die Anzahl der Einträge, die Version, die letzte Änderung und ob es weiterhin **keine PIN** hat.

> **Die Profil-ID wird aus dem Namen abgeleitet und lautet nicht zwingend `dad`.** ASCII-Buchstaben und Ziffern werden zu einem Slug (`Dad` → `dad`, `nas box` → `nas-box`); ein Name ohne nutzbare ASCII-Zeichen fällt in die Reihe `home` / `home-2`. Ein auf `…:dad` beschränktes Token passt dann zu keinem Profil und erhält einen 403-Fehler — passe den Scope des Tokens an die tatsächliche ID an (empfohlen) oder gib dem ersten Profil einen ASCII-Namen.

Der Host-Port ist standardmäßig 7600. Auf manchen Windows-Rechnern belegt Hyper-V einen Portbereich (auf der Testmaschine 7532–7631); schlägt `up` fehl, wähle einen freien Port außerhalb dieses Bereichs: `LAUNCHBAY_HOST_PORT=8901 docker compose up -d` (der Port innerhalb des Containers bleibt unverändert). Projekt- und Containername sind fest als `launchbay` vorgegeben, sodass `docker logs launchbay` und `docker exec launchbay …` unabhängig vom Verzeichnis des Klons funktionieren.

### Browser-Erweiterung (optional)

`extension/` ist eine Chrome- / Edge-Erweiterung (Manifest V3): Klicke mit der rechten Maustaste auf die aktuelle Seite, um sie in einem bestimmten Profil zu speichern, oder speichere alle Tabs des Fensters auf einmal. Sie ist nicht im Docker-Image enthalten und wird separat installiert.

Sie benötigt ein profilgebundenes Token, da sie das Sitzungscookie nicht auslesen kann. Trage `LAUNCHBAY_WRITE_TOKENS` ein, starte neu und gehe wie folgt vor:

1. Öffne `chrome://extensions` und aktiviere den **Entwicklermodus**
2. Klicke auf **Entpackte Erweiterung laden** und wähle den Ordner `extension/`
3. Öffne die Optionen der Erweiterung und trage die Serveradresse (ohne abschließenden Schrägstrich), die Standard-Profil-ID und das Secret des Tokens ein
4. Klicke auf **Diese Maschine testen**

Ein 403-Fehler beim Test ist kein Netzwerkproblem: Er bedeutet, dass Token und Profil-ID nicht übereinstimmen. Korrigiere den Scope in `LAUNCHBAY_WRITE_TOKENS`, starte neu und teste erneut.

Nach der Installation: Rechtsklick auf eine beliebige Seite → Speichern. Über die Optionsseite lässt sich auch der gesamte Lesezeichenbaum des Browsers importieren (die Berechtigung für Lesezeichen wird erst beim ersten Aufruf angefragt). Ein Import ist auf allen drei Wegen (Web, Erweiterung, CLI) nur in ein **leeres** Profil möglich — er ersetzt die Lesezeichen des Profils vollständig, anstatt sie anzuhängen. Chrome warnt mit „Daten auf allen Websites lesen und ändern“, weil die Erweiterung `http://*/*` und `https://*/*` benötigt, um auf den Rechtsklick zu reagieren; das Token verbleibt in `chrome.storage.local` und wird niemals synchronisiert. Firefox ist bisher ungeprüft.

### Dateiberechtigungen für das Datenverzeichnis

Die Daten landen in `./data` neben der Datei `docker-compose.yml` und entsprechen innerhalb des Containers immer `/data`. Um den Speicherort zu ändern, passe die Zeile unter `volumes` an:

```yaml
    volumes:
      - /volume1/docker/launchbay:/data
```

Unter Windows kollidiert ein absoluter Pfad wie `D:` mit dem Trennzeichen; nutze hier die ausführliche Syntax:

```yaml
    volumes:
      - type: bind
        source: D:/launchbay-data
        target: /data
```

Der Container läuft unter der **UID 65534**, während das Hostverzeichnis meist root gehört — das Erstellen von Profilen, Importe und Schreibvorgänge schlagen dann fehl. Übertrage die Rechte auf dem Hostrechner an diesen Benutzer:

```bash
sudo chown -R 65534:65534 ./data
```

Verwende als Workaround nicht `--user root`: Damit könnte der Dienst jede Datei auf deinem System mit Root-Rechten überschreiben. Der Healthcheck erstellt und löscht bei jeder Überprüfung eine Testdatei; falsche Berechtigungen führen daher in `docker ps` zum Status **unhealthy**, und im Log erscheint derselbe chown-Befehl. Die Einrichtungsseite weist auf dasselbe hin (`UID 65534` im Container; dein eigener Benutzer, wenn du die Binärdatei direkt ausführst).

## Bedienung

| Was du tun möchtest | Wo |
|---|---|
| Aussehen der Seite, Ansicht, Gruppen, Hintergrund, Adressmodus oder Öffnungsverhalten ändern | Einstellungen in der oberen Leiste oder `POST /prefs`, `/prefs/background`, `/prefs/group`, `/prefs/tabs` |
| Eigene PIN ändern | Meine PIN im eigenen Einstellungs-Panel oder `POST /prefs/pin` / `POST /api/v1/profiles/self/pin` |
| Profil erstellen/löschen, PIN einer Person setzen, jemanden deaktivieren | `/admin` (oder der Link im Fußbereich der Auswahlliste). Beim Löschen wird der Schlüssel erneut abgefragt; in der CLI stehen für die Profilverwaltung nur `profile create` und `profile rebuild` bereit |
| Ein weiteres Profil für dich erstellen | Der einklappbare Bereich „Neues für dich“ auf der Auswahlliste oder `POST /api/v1/profiles/new` (ohne Zugangsdaten) |
| Namen bereinigen, die eigentlich URLs sind | `launchbay profile fix-names --profile <id>` (zuerst mit `--dry-run` testen) |
| Lesezeichen durch Skripte oder KI ändern | `POST /api/v1/batch` mit `expectedRev` |
| Beschädigtes Profil reparieren | `POST /api/v1/profiles/rebuild` oder `launchbay profile rebuild` |
| Jede durchgeführte Änderung einsehen | `GET /api/v1/history` (nur Aktionen und Pfade, niemals Inhalte) |
| Ein ganzes Profil sichern oder umziehen | `GET /api/v1/backup` / `POST /api/v1/restore`, `launchbay backup` / `restore` oder Backup & Verlauf in den Einstellungen |

Weitere Maschinenrouten: `POST /api/v1/usage` zählt Klicks, `POST /api/v1/resolve` wandelt eine URL in Titel und Icon um, und `GET /api/v1/status` liefert die profilübergreifende Zusammenfassung für den Admin.

Backups schließen PIN, Profilidentität und Hintergrundbild aus. Ein Backup mit Lesezeichen kann nur in ein leeres Profil eingespielt werden; ein reines Einstellungs-Backup kann jederzeit importiert werden. Beim Import wird die Versionsnummer unter der Profilsperre geprüft; bei einem Versionskonflikt antwortet die API mit 409 `[rev-conflict]` und gibt die `currentRev` zurück. Auf diese Weise geschriebene Einstellungen erhöhen die Revisionsnummer nicht und erscheinen nicht unter „Zurücksetzen“.

**Die vollständige Liste der Endpunkte, Request-Bodies und Fehlercodes ist in [AGENTS.md](../../AGENTS.md) dokumentiert** — dem verbindlichen Vertrag für KI-Agenten, der per Testsuite gegen die Routing-Tabelle abgeglichen wird. Er wird hier nicht wiederholt.

**Gewohnheiten der CLI**: Ausgaben gehen nach stdout, Fehler nach stderr, und jede Fehlermeldung zitiert den Code des Servers (`[rev-conflict]`, `[ops-invalid]`). `import-bookmarks` nimmt die Datei als Positionsargument entgegen und führt den Schreibvorgang im selben Durchlauf wie die Berichterstellung aus (`--dry-run` stoppt vor dem Schreiben); Backups und Exporte streamen die vollständige Antwort, sodass Dateien über 4 MiB kein Problem darstellen. `--help` und `-h` beenden mit Exit-Code 0, Anwendungsfehler mit 2, sonstige Fehler mit 1. `revert --to 0` wird als Anwendungsfehler abgewiesen und gibt den exakten `curl`-Befehl aus, mit dem dies möglich ist.

**Null JavaScript ist das Fundament, nicht die Obergrenze.** Die Suche ist ein HTML-Formular, Profilerstellung und PIN-Eingabe sind native Formular-POSTs, und bei deaktivierten Skripten bleibt jedes Lesezeichen sichtbar und anklickbar. Integrierte Sprunglinks führen direkt zum Hauptinhalt. Eine einzelne Erweiterungsebene ergänzt das Hinzufügen-Formular, Inline-Bearbeitung, Drag-and-Drop-Sortierung, die ⌘K-Palette und Offline-Unterstützung; fällt diese Ebene aus, bleibt der Weg ohne JavaScript stets verfügbar.

## Grenzen

- **Nur im lokalen Netzwerk, kein HTTPS.** Lesezeicheninhalte liegen als Klartext-JSON auf der Festplatte; schalte einen TLS-Reverse-Proxy davor, bevor du den Dienst öffentlich zugänglich machst.
- **Eine beschädigte `profile.json` sperrt das jeweilige Profil.** `revert` kann dies nicht beheben; `POST /api/v1/profiles/rebuild` (oder `launchbay profile rebuild`) schreibt ausschließlich diese Datei neu und rührt Lesezeichen, Schnappschüsse oder Verlauf mit keinem Byte an.
- **Icon-Abruf hat eine Abklingzeit**: Ein fehlgeschlagener Origin wird 24 Stunden lang nicht erneut angefragt. Diese 24-stündige Pause verhindert, dass eine nicht erreichbare Website fortlaufend sondiert wird.
- **Keine geteilten Listen**: Du kannst kein Lesezeichen für die ganze Familie freigeben oder eine Seite veröffentlichen. Dass jeder seine eigenen Daten behält, ist der eigentliche Zweck.
- **Bisher nicht auf einem echten NAS validiert.** Alle Abnahmerunden liefen unter Windows + Docker Desktop; das LCP-Ziel im LAN, der Kaltstart-LCP, die Scroll-Bildrate bei 1.000 Lesezeichen und die Latenz bei Offline-Cache-Treffern sind noch ungemessen.
- **Keine Wetter- oder Uhr-Widgets.** Sie liegen außerhalb des Zwecks dieser Seite. Fußzeilentexte und Lesezeicheninhalte laufen über `POST /api/v1/batch`; die Dateien auf der Platte unterliegen dem Server.
- **Die Oberflächensprachen decken die Benutzeroberfläche ab, nicht skriptbezogene Texte.** API-Fehlerangaben (`detail`), Feldvalidierungsgründe und der von Skripten gelesene Importbericht bleiben auf Chinesisch (der Bericht im Webbrowser folgt der Seitensprache). Aufrufer benötigen stabile, vergleichbare Texte; Lesezeichentitel, Gruppennamen und URLs sind Inhalte und werden ebenfalls nicht übersetzt.
- **Tokens sind standardmäßig leer**, daher sind Erweiterung und CLI deaktiviert, bis du eines hinzufügst; beim Admin-Schlüssel ist es genau umgekehrt: Er ist zum Starten zwingend erforderlich.

## Woher das Image kommt

compose lädt ausschließlich veröffentlichte Images herunter; das Bauen übernimmt die CI. Wird ein Tag `v*` gesetzt (oder der Workflow manuell ausgelöst), baut `.github/workflows/docker.yml` Images für amd64 und arm64 und überträgt sie zu Docker Hub und GHCR. Release-Tags lauten `<version>`, `<major>.<minor>` und `<major>` (aktuell `0.4.0` / `0.4` / `0`), ergänzt durch `latest` und den Commit-SHA; die Image-Version stammt ausschließlich aus dem Git-Tag.

Wenn du das Repository nicht klonen möchtest, kannst du das Image direkt starten:

```bash
docker run -d --name launchbay -p 7600:7600 \
  -v launchbay-data:/data -e LAUNCHBAY_DATA_DIR=/data \
  -e LAUNCHBAY_ADMIN_TOKEN="dein-eigener-admin-schluessel" \
  -e LAUNCHBAY_WRITE_TOKENS="ai-dad:dein-eigener-schluessel:dad" -e TZ=Asia/Shanghai \
  --health-cmd "/launchbay healthcheck" --health-interval 30s \
  ghcr.io/rockbenben/launchbay:latest
```

## Über den 365 Open Source Plan

Projekt **#042** des [365 Open Source Plan](https://github.com/rockbenben/365opensource) — eine Person + KI, über 300 Open-Source-Projekte in einem Jahr.

[Reiche deine Idee ein →](https://365.aishort.top/) · [Discord](https://discord.gg/PZTQfJ4GjX) · [Telegram](https://t.me/aishort_top)
