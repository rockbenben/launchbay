# launchbay

> A self-hosted bookmark dashboard for your NAS, router, and browser bookmarks.

[![License: MIT](https://img.shields.io/badge/License-MIT-1f6feb)](LICENSE) [![365 Open Source Plan #042](https://img.shields.io/badge/365%20Open%20Source%20Plan-%23042-1f6feb)](https://github.com/rockbenben/365opensource)

[▶ Quick start](#quick-start) · [Docker image](https://github.com/rockbenben/launchbay/pkgs/container/launchbay) · [docker-compose.yml](https://raw.githubusercontent.com/rockbenben/launchbay/refs/heads/main/docker-compose.yml)

**English** · [简体中文](README.zh.md) · [繁體中文](docs/i18n/README.zh-Hant.md) · [日本語](docs/i18n/README.ja.md) · [한국어](docs/i18n/README.ko.md) · [Español](docs/i18n/README.es.md) · [Português](docs/i18n/README.pt.md) · [Français](docs/i18n/README.fr.md) · [Deutsch](docs/i18n/README.de.md) · [Italiano](docs/i18n/README.it.md) · [Русский](docs/i18n/README.ru.md) · [العربية](docs/i18n/README.ar.md) · [हिन्दी](docs/i18n/README.hi.md) · [বাংলা](docs/i18n/README.bn.md) · [ไทย](docs/i18n/README.th.md) · [Türkçe](docs/i18n/README.tr.md) · [Tiếng Việt](docs/i18n/README.vi.md) · [Bahasa Indonesia](docs/i18n/README.id.md)

![launchbay profile page: top-bar search, sidebar groups and tile cards](docs/home-1280.en.webp)

Make launchbay your browser homepage for the sites and devices you open every day. Each person can have a separate profile and PIN on the same server.

## What it covers

| Aspect | Whether it works for you |
|---|---|
| How you run it | Docker image for Linux amd64 and arm64, or build the standalone binary |
| Where it runs | A home NAS, Raspberry Pi, soft router, or one box on the company intranet |
| Who uses it | One person, a family, or a small team — one profile each; the admin handles accounts, not content |
| Browsers | Any modern browser; with JavaScript off every bookmark is still visible and clickable |
| What you install | Docker Compose for the published image. Data lives in files. |
| Where the data is | One ordinary directory, `data/`; a backup is copying it |
| Interface language | 18 interface languages, listed above. Each profile can choose its own. |

## What it does

**Saving and tidying**

- **Drop it and it is saved**: drag a link from the bookmarks bar, the address bar, or another tab onto a group. No dialog — the drop is the answer.
- **Drag to reorder** inside a group, across groups, or onto a group title to append. Long-press or a shortcut starts multi-select, then tag, move or delete in one go.
- **Save from any page**: drag the bookmarklet to your bookmarks bar; one click brings the address back, one more press saves it.
- **Import from your browser**: upload the bookmark file Chrome / Edge / Firefox exported and get a per-entry report — what got in, what did not, and why. The web page can import straight away or show the report first; the CLI writes in the same run unless you pass `--dry-run`.
- **Take a whole profile with you**: one file holds the bookmarks **and** the page's look (theme, layout, view, second line, page name, language, address mode). The PIN, the profile's identity and the background image are deliberately excluded.

**Finding it**

- **Groups and search**: the top bar searches names, addresses, notes and tags; press Enter and the server answers — no JavaScript needed. With scripts on, press `/` or use the `⌘/Ctrl K` palette.
- **Featured groups**: promote the groups you use most into a large-card band at the top.
- **10,000+ icons, offline**: Tabler and Simple Icons are gzipped into the binary; site icons are served from the local cache and can be refreshed when adding or editing a bookmark.

**At home and away**

- **Two addresses per bookmark**: set a primary and an optional WAN address. "Auto" follows the address you opened launchbay on; Settings → Bookmark addresses overrides it for this browser. Search and copy use the active address, and bookmarks without a WAN address keep the primary one. Behind a reverse proxy, preserve the original `Host`.

**The page itself**

- **One settings panel**: top bar → Settings. It holds the page's look and language, the group list, backup and history, and **My PIN** — lock or unlock your own profile without the admin.
- **Works offline**: the last page is cached, and bookmarks added while offline — or while the machine is down — queue up and replay when the browser reconnects; press **Send now** if the server itself was down.
- **Scripts and AI have their own keys**: a REST API plus profile-scoped tokens for the extension, the CLI and automation. A token that steps outside its profile gets a 403.
- Restore earlier bookmark versions from **Settings → Backup & history**.

## Quick start

```bash
git clone https://github.com/rockbenben/launchbay
cd launchbay
```

> [!IMPORTANT]
> Set your own `LAUNCHBAY_ADMIN_TOKEN` under `environment` in `docker-compose.yml` before starting the service. Anyone on your LAN who knows the public default key can create or delete profiles and reset PINs.

On Linux or a NAS, prepare the data directory first.

```bash
mkdir -p ./data
sudo chown 65534:65534 ./data
```

```bash
docker compose up -d
```

On the host, open [http://localhost:7600](http://localhost:7600). From another device on the LAN, use `http://<LAN-IP>:7600`. Create a profile with a 4–6 digit PIN, open it, and select **Add** to save your first bookmark.

To update an existing installation, run `docker compose pull` and then `docker compose up -d`.

Use **No PIN** to let anyone on your LAN open that profile. Open `/admin` and enter your admin key to manage profiles.

### Admin key and tokens

| Variable | Required? | What it covers |
|---|---|---|
| `LAUNCHBAY_ADMIN_TOKEN` | **Required**, a bare key | Accounts only: create/delete profiles, set any PIN, disable/enable, cross-profile status and history. Every content endpoint (`/api/v1/data`, `/batch`, `/export`, …) answers 403 — it cannot read bookmarks |
| `LAUNCHBAY_WRITE_TOKENS` | Optional, empty by default | Profile-scoped write tokens, `name:secret:profile`. Only the browser extension, the CLI (`revert` / `import-bookmarks` / `backup` / `restore` / `profile fix-names`) and AI or scripts need one |

> **Why empty by default?** The bookmark page, admin, revert and import all ride on the session cookie; no token is needed. The three things that do need one are installed on purpose. Shipping a default would give every machine a free key able to write the `dad` profile (`dad` being the most common profile id).

A profile has no "is admin" attribute. The admin page shows account-level actions plus each profile's item count, version, last change and whether it still has **no PIN**.

> **The profile id is derived from its name, and is not necessarily `dad`.** ASCII letters and digits become a slug (`Dad` → `dad`, `nas box` → `nas-box`); a Chinese name has nothing usable to slug, so it falls into the `home` / `home-2` sequence. A token scoped to `…:dad` then matches no profile and gets a 403 — change the token's scope to the real id (recommended), or give the first profile an ASCII name.

The host port defaults to 7600. On some Windows machines Hyper-V holds a block of ports (7532–7631 on the machine this was tested on); if `up` fails, pick a free port outside it: `LAUNCHBAY_HOST_PORT=8901 docker compose up -d` (the container port does not move). The project and container names are hardcoded as `launchbay`, so `docker logs launchbay` and `docker exec launchbay …` work regardless of the clone directory.

### Browser extension (optional)

`extension/` is a Chrome / Edge extension (Manifest V3): right-click the current page to save it into a named profile, or save every tab in the window at once. It is not in the image; install it yourself.

It needs a profile-scoped token, because it cannot read the session cookie. Fill in `LAUNCHBAY_WRITE_TOKENS`, restart, then:

1. Open `chrome://extensions`, turn on **Developer mode**
2. **Load unpacked**, pick `extension/`
3. Open the extension's options and fill in the machine address (no trailing slash), the default profile id, and the secret from the token
4. **Test this machine**

The connection test distinguishes a disabled profile, a mismatched profile ID, and an admin key used in place of a profile token.

Once installed: right-click any page → save; the options page can also import the whole bookmark tree, asking for the bookmarks permission on first use only. An import goes only into an **empty** profile on all three routes (web, extension, CLI) — it replaces the profile's bookmarks rather than adding to them. Chrome warns "Read and change your data on all sites" because the extension needs `http://*/*` and `https://*/*` to answer a right-click; the token stays in `chrome.storage.local`, never synced. Firefox is unverified.

### Data directory permissions

Data lands in `./data` next to `docker-compose.yml`, and is always `/data` inside the container. To move it, change the `volumes` line:

```yaml
    volumes:
      - /volume1/docker/launchbay:/data
```

On Windows, an absolute path like `D:` collides with the separator; use the long syntax:

```yaml
    volumes:
      - type: bind
        source: D:/launchbay-data
        target: /data
```

The container runs as **uid 65534**, while the host directory is usually root-owned — profile creation, import and writes then fail. Hand it to that user on the host:

```bash
sudo chown -R 65534:65534 ./data
```

Do not work around it with `--user root`: that lets the service write every file in your house as root. The healthcheck creates and deletes a probe file on every check, so wrong permissions show up as **unhealthy** in `docker ps`, and the log prints the same chown command. The setup page says the same thing (`uid 65534` in a container; your own user if you run the binary directly).

## How to use it

| What you want | Where |
|---|---|
| Change the page's look, view, groups, background, address mode, or how a bookmark opens | Settings in the top bar, or `POST /prefs`, `/prefs/background`, `/prefs/group`, `/prefs/tabs` |
| Change your own PIN | My PIN in your own panel, or `POST /prefs/pin` / `POST /api/v1/profiles/self/pin` |
| Create or delete a profile, set someone's PIN, disable someone | `/admin` (or the footer link on the selection page). Deleting asks for the key again; for profile administration the CLI only covers `profile create` and `profile rebuild` |
| Create one more for yourself | The collapsed "New one for yourself" on the selection page, or `POST /api/v1/profiles/new` (no credential) |
| Clean up names that are really addresses | `launchbay profile fix-names --profile <id>` (add `--dry-run` first) |
| Let AI or a script change bookmarks | `POST /api/v1/batch` with `expectedRev` |
| Rescue a broken profile | `POST /api/v1/profiles/rebuild`, or `launchbay profile rebuild` |
| See what every change was | `GET /api/v1/history` (actions and paths only, never content) |
| Back up or move a whole profile | `GET /api/v1/backup` / `POST /api/v1/restore`, `launchbay backup` / `restore`, or Backup & history in Settings |

Other machine routes: `POST /api/v1/usage` counts clicks, `POST /api/v1/resolve` turns a URL into a title and icon, and `GET /api/v1/status` is the admin's cross-profile summary.

Backups exclude the PIN, the profile's identity and the background image. A backup with bookmarks goes only into an empty profile; a settings-only backup can go anywhere. Imports check the revision under the profile lock and answer 409 `[rev-conflict]` with `currentRev` on a race. Settings written this way do not move the revision and do not appear in Revert.

**The full endpoint list, request bodies and error codes are in [AGENTS.md](AGENTS.md)** — the authoritative contract for AI, checked against the route table by tests. It is not duplicated here.

**CLI habits**: results go to stdout, failures to stderr, and every failure quotes the server's code (`[rev-conflict]`, `[ops-invalid]`). `import-bookmarks` takes the file as a positional argument and writes in the same run as it prints the report (`--dry-run` stops before writing); backups and exports stream the full response, so files above 4 MiB are fine. `--help` and `-h` exit 0, usage errors 2, other failures 1. `revert --to 0` is refused as a usage error and prints the exact `curl` that does it.

**Zero JavaScript is the floor, not the ceiling.** Search is a form, profile creation and PINs are native form posts, and with scripts off every bookmark is still visible and clickable. Built-in skip links jump straight to the content. One enhancement layer adds the add sheet, in-place editing, drag sorting, the ⌘K palette and offline support; if it breaks, the zero-JS route is still there.

## Limits

- **LAN only, no HTTPS.** Bookmark contents are plain JSON on disk; put a TLS reverse proxy in front before exposing it.
- **A broken `profile.json` locks that profile.** `revert` cannot fix it; `POST /api/v1/profiles/rebuild` (or `launchbay profile rebuild`) rewrites only that file and touches no bookmark, snapshot or history byte.
- **Icon fetching has a cooldown**: automatic retries pause for 24 hours after a failure. Use **Refresh site icon** to retry immediately.
- **No shared list**: you cannot share one bookmark with the family or publish a page. Each keeping their own is the point.
- **Not yet validated on a real NAS.** All acceptance rounds ran on Windows + Docker Desktop; the LAN LCP target, cold-start LCP, 1,000-bookmark scroll frame rate and offline cache-hit latency are still unmeasured.
- **No weather or clock widgets.** They are outside what this page is for. Footer text and bookmark content go through `POST /api/v1/batch`; the files belong to the server.
- **The interface languages cover the chrome, not script-facing text.** API error `detail`, field-validation reasons and the import report as read by a script stay Chinese (the web page's report follows the page language). Callers want stable, comparable text; your own bookmark titles, group names and URLs are content and are not translated either.
- **Tokens are empty by default**, so the extension and CLI are off until you add one; the admin key is the other way round and is required to start.

## Where the image comes from

compose only pulls a published image; building is CI's job. Tag a `v*` (or dispatch the workflow), and `.github/workflows/docker.yml` builds amd64 and arm64 for Docker Hub and GHCR. Release tags are `<version>`, `<major>.<minor>` and `<major>`, plus `latest` and the commit sha; the image version comes only from the git tag.

To skip the clone, pull the image directly:

```bash
docker run -d --name launchbay -p 7600:7600 \
  -v launchbay-data:/data -e LAUNCHBAY_DATA_DIR=/data \
  -e LAUNCHBAY_ADMIN_TOKEN="your-own-admin-key" \
  --health-cmd "/launchbay healthcheck" --health-interval 30s \
  ghcr.io/rockbenben/launchbay:latest
```

## About the 365 Open Source Plan

Project **#042** of the [365 Open Source Plan](https://github.com/rockbenben/365opensource) — one person + AI, 300+ open-source projects in a year.

[Submit your idea →](https://365.aishort.top/) · [Discord](https://discord.gg/PZTQfJ4GjX) · [Telegram](https://t.me/aishort_top)
