# launchbay

> Every bookmark you forgot you saved, on one page — self-hosted, offline, no sign-up.

[![License: MIT](https://img.shields.io/badge/License-MIT-1f6feb)](LICENSE) [![365 Open Source Plan #042](https://img.shields.io/badge/365%20Open%20Source%20Plan-%23042-1f6feb)](https://github.com/rockbenben/365opensource) [![ci](https://img.shields.io/github/actions/workflow/status/rockbenben/launchbay/ci.yml?label=ci&branch=main)](https://github.com/rockbenben/launchbay/actions/workflows/ci.yml)

[▶ Quick start](#quick-start) · [⬇ Releases](https://github.com/rockbenben/launchbay/releases)

**English** · [简体中文](README.zh.md) · [繁體中文](docs/i18n/README.zh-Hant.md) · [日本語](docs/i18n/README.ja.md) · [한국어](docs/i18n/README.ko.md) · [Español](docs/i18n/README.es.md) · [Português](docs/i18n/README.pt.md) · [Français](docs/i18n/README.fr.md) · [Deutsch](docs/i18n/README.de.md) · [Italiano](docs/i18n/README.it.md) · [Русский](docs/i18n/README.ru.md) · [العربية](docs/i18n/README.ar.md) · [हिन्दी](docs/i18n/README.hi.md) · [বাংলা](docs/i18n/README.bn.md) · [ไทย](docs/i18n/README.th.md) · [Türkçe](docs/i18n/README.tr.md) · [Tiếng Việt](docs/i18n/README.vi.md) · [Bahasa Indonesia](docs/i18n/README.id.md)

![launchbay profile page: top-bar search, sidebar groups and tile cards](docs/home-1280.en.webp)

Put the addresses you can never remember onto one page: the NAS admin panel, the router, the company OA, the monitoring dashboard. Make it your browser home page — **one click, no spinner.**

It runs on your own machine — a home NAS, a Raspberry Pi, a soft router, one box on the company intranet — so your bookmarks never touch somebody else's server.

## Why launchbay

Most bookmark tools differ on four things: whose server holds the data, how a household shares it, whether it works offline, and whether you can organise it by dragging.

| Compared on | Browser's own bookmarks | Cloud bookmark service | launchbay |
|---|---|---|---|
| Where the data is | Your browser profile, following your account | The vendor's servers | **Your own machine** |
| One per person | Via browser profiles | Per account | **One each, kept apart on the server** |
| Offline | Local only | Doesn't work | **Cache + an offline queue** |
| Organising by dragging | A folder tree, not a laid-out page | Mostly not | **Drop to save, drag to reorder** |
| Who else can read it | Your browser account | The vendor | **Nobody through the app** |

No account, no third-party script, no telemetry. The only outside requests are fetching the page you add (for its title) and its favicon; a failed icon fetch cools down for 24 hours.

**If you are just one person, the browser's own bookmarks page is probably enough.** launchbay trades that for three things:

- **Opening it doesn't wait.** The page is server-rendered: bookmarks and their icons are in the HTML before any script runs. The target is LAN first-paint LCP < 300ms; a cold start on a real NAS has not been measured yet (see [Limits](#limits)).
- **One each, and nobody else sees it.** Each profile can have its own 4–6 digit PIN. The admin can create, delete and reset PINs, but every content endpoint refuses the admin key with a 403 — there is **no route** to your bookmarks. This is not cryptography against the admin or the LAN (anyone on the LAN can edit a cookie); it is the server keeping profiles apart.
- **Breaking it is reversible.** Every bulk change takes a snapshot first. Revert shows how many bookmarks and which groups each older version had, and saves the current version before restoring — so a mistaken revert can itself be reverted. History is kept, never erased: each line says who touched this profile, when, through what (the page, AI, the extension, the CLI), and which entry — never the content.

## What it covers

| Aspect | Whether it works for you |
|---|---|
| How you run it | `docker compose up -d`, or the static binary (amd64 / arm64) |
| Where it runs | A home NAS, Raspberry Pi, soft router, or one box on the company intranet |
| Who uses it | One person, a family, or a small team — one profile each; the admin handles accounts, not content |
| Browsers | Any modern browser; with JavaScript off every bookmark is still visible and clickable |
| What you install | Docker only: no third-party libraries, no database, no Node at build time |
| Where the data is | One ordinary directory, `data/`; a backup is copying it |
| Interface language | 4 interface languages (Simplified Chinese, Traditional Chinese, English, Arabic RTL); pick the machine default on setup, then each profile picks its own |

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
- **10,000+ icons, offline**: Tabler and Simple Icons are gzipped into the binary; a site's favicon is fetched once and then served from the local cache.

**At home and away**

- **Two addresses per bookmark**: set a primary and an optional WAN address. "Auto" follows the address you opened launchbay on; Settings → Bookmark addresses overrides it for this browser. Search and copy use the active address, and bookmarks without a WAN address keep the primary one. Behind a reverse proxy, preserve the original `Host`.

**The page itself**

- **One settings panel**: top bar → Settings. It holds the page's look and language, the group list, backup and history, and **My PIN** — lock or unlock your own profile without the admin.
- **Works offline**: the last page is cached, and bookmarks added while offline — or while the machine is down — queue up and replay when the browser reconnects; press **Send now** if the server itself was down.
- **Scripts and AI have their own keys**: a REST API plus profile-scoped tokens for the extension, the CLI and automation. A token that steps outside its profile gets a 403.

## Quick start

```bash
git clone https://github.com/rockbenben/launchbay
cd launchbay
docker compose up -d
```

No `.env`, no Go toolchain — the image is simply pulled.

Open **http://<this machine's LAN address>:7600** from any device on the LAN — **not `localhost`**, which means "this device" and will open the phone you typed it on. Sitting at the machine itself, `http://localhost:7600` works.

The image is `:latest`; the first `up` pulls the newest release. A machine that is already running keeps its local image, so run `docker compose pull` first to update.

> **Change the default admin key first.** compose ships `launchbay123`, which is public. Anyone on your LAN who knows it can create or delete profiles and reset PINs (they still cannot read bookmarks). Put your own in `LAUNCHBAY_ADMIN_TOKEN` and run `docker compose up -d` again:
>
> ```yaml
> LAUNCHBAY_ADMIN_TOKEN: "your-own-admin-key"
> ```

Opening it takes you into the one-time setup page: name the first profile and set a 4–6 digit PIN. There are no default credentials, and the page closes for good once any profile exists.

**"No PIN" is a separate tick, not an empty box.** Leaving the PIN empty is rejected; ticking **No PIN** creates a profile open to everyone on the LAN. That is deliberate — the expensive mistake is typing three digits and hitting enter, which would quietly publish one person's bookmarks to the house. A profile without a PIN shows no PIN box on the selection page. You can add or remove one later in **My PIN**; removing it needs the old PIN only while one is set.

The first profile is **not an admin**. The machine is managed with `LAUNCHBAY_ADMIN_TOKEN`: open `/admin`, type the key once, and you get a cookie that belongs to the admin identity alone (30 days).

**Later people do not wait for the admin.** The selection page has a collapsed **"New one for yourself"**: open it, give a name and PIN, and the profile exists (`POST /new`; the machine route is `POST /api/v1/profiles/new`). It needs no credential, so any device on the LAN can create one; the card appears, and the admin can delete it at any time. It creates an **empty** profile that belongs to its own PIN and grants no access to anyone else's bookmarks. Names take up to 60 characters, and reusing someone else's name makes the two cards indistinguishable.

**Deleting a profile is the one irreversible action**, so it asks for the machine key again: the confirmation page spells out how many bookmarks and snapshots go away, and you type `LAUNCHBAY_ADMIN_TOKEN` once more. The machine path (`DELETE /api/v1/profiles`) accepts only that key; an admin cookie alone is a 403. Snapshots and private backgrounds go with the profile, so a revert cannot save it.

The same page has an **Interface language** dropdown; pick it while setting up. It sets the **machine-wide default**; afterwards everyone can pick their own in their own panel, and the admin's change only reaches people who have not picked one.

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

A 403 from the test is not a network failure: it means the token and the profile id do not match. Fix the scope in `LAUNCHBAY_WRITE_TOKENS`, restart, test again.

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
- **Icon fetching has a cooldown**: a failed origin is not retried for 24 hours. The 24-hour cooldown keeps a failed site from being probed again and again.
- **No shared list**: you cannot share one bookmark with the family or publish a page. Each keeping their own is the point.
- **Not yet validated on a real NAS.** All acceptance rounds ran on Windows + Docker Desktop; the LAN LCP target, cold-start LCP, 1,000-bookmark scroll frame rate and offline cache-hit latency are still unmeasured.
- **No weather or clock widgets.** They are outside what this page is for. Footer text and bookmark content go through `POST /api/v1/batch`; the files belong to the server.
- **The interface languages cover the chrome, not script-facing text.** API error `detail`, field-validation reasons and the import report as read by a script stay Chinese (the web page's report follows the page language). Callers want stable, comparable text; your own bookmark titles, group names and URLs are content and are not translated either.
- **Tokens are empty by default**, so the extension and CLI are off until you add one; the admin key is the other way round and is required to start.

## Where the image comes from

compose only pulls a published image; building is CI's job. Tag a `v*` (or dispatch the workflow), and `.github/workflows/docker.yml` builds amd64 and arm64 for Docker Hub and GHCR. Release tags are `<version>`, `<major>.<minor>` and `<major>` (today `0.4.0` / `0.4` / `0`), plus `latest` and the commit sha; the image version comes only from the git tag.

To skip the clone, pull the image directly:

```bash
docker run -d --name launchbay -p 7600:7600 \
  -v launchbay-data:/data -e LAUNCHBAY_DATA_DIR=/data \
  -e LAUNCHBAY_ADMIN_TOKEN="your-own-admin-key" \
  -e LAUNCHBAY_WRITE_TOKENS="ai-dad:your-own-key:dad" -e TZ=Asia/Shanghai \
  --health-cmd "/launchbay healthcheck" --health-interval 30s \
  ghcr.io/rockbenben/launchbay:latest
```

## About the 365 Open Source Plan

Project **#042** of the [365 Open Source Plan](https://github.com/rockbenben/365opensource) — one person + AI, 300+ open-source projects in a year.

[Submit your idea →](https://365.aishort.top/) · [Discord](https://discord.gg/PZTQfJ4GjX) · [Telegram](https://t.me/aishort_top)
