# whatevermp4

A self-hosted watch-party app. Upload videos, add torrents or import `.m3u8` streams; a worker transcodes everything to HLS, and people watch together in rooms that stay in sync: play, pause, seek and speed are shared, and the room waits for anyone who is buffering. There are no accounts: everyone just picks a nickname.

**Highlights**

- Library with nested folders (a series → its seasons), search and filters, drag-and-drop uploads, bulk actions and a storage overview
- Uploads go straight from the browser to S3 storage in 64 MB parts; torrents and `.m3u8` links download on the server
- Transcoding to adaptive HLS (source quality plus up to two lower ones), subtitles, posters and seek-bar previews, with optional NVIDIA (NVENC) hardware encoding
- Rooms with a play queue (autoplays the next episode), chat, per-viewer ping and sync status, host hand-over, and private rooms with an invite link or password
- Live updates everywhere over WebSockets; per-viewer resume positions, quality and volume

## Contents

- [How it is built](#how-it-is-built)
- [Run it with Docker](#run-it-with-docker)
- [Let other people join](#let-other-people-join)
- [Using it](#using-it)
- [Developing](#developing)
- [Configuration reference](#configuration-reference)
- [Troubleshooting](#troubleshooting)

## How it is built

| Service | What it is | Port |
| --- | --- | --- |
| `web` | SvelteKit frontend (Node) | 3000 |
| `movie` | Go API: REST, room WebSockets, live-update events, HLS playback proxy | 8088 |
| `worker` | Go job runner: transcoding (ffmpeg), torrent and `.m3u8` downloads, `/healthz` + `/metrics` | 8089 (internal only) |
| `db` | PostgreSQL 16 | 5432 |
| `rustfs` | S3-compatible object storage for originals and HLS output | 9044 (console 9045) |

The database schema is created and upgraded automatically when the API or worker starts. Developer notes on the architecture and conventions are in [CLAUDE.md](CLAUDE.md).

## Run it with Docker

You need [Docker](https://docs.docker.com/get-docker/) with Compose (Docker Desktop on Windows and macOS).

```bash
git clone <this repository> whatevermp4
cd whatevermp4
cp .env.example .env
docker compose up --build -d
```

Open **http://localhost:3000**, pick a nickname, and add a movie from the **Library**.

The first build takes a few minutes. Data lives in Docker volumes (`pgdata` for the database, `movies_data` for media, `worker_work` for the worker's scratch space), so it survives restarts and rebuilds.

```bash
docker compose ps                # health of each service
docker compose logs -f worker    # transcoding / download progress
docker compose down              # stop (keeps your data)
docker compose down -v           # stop and DELETE all data
```

To update after pulling new code, run `docker compose up --build -d` again; database migrations apply on startup.

### GPU transcoding (NVIDIA)

Transcoding uses the CPU (libx264) by default. With an NVIDIA GPU it can use NVENC, which is several times faster:

- Linux: install the [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html)
- Windows: Docker Desktop with the WSL 2 backend and a current NVIDIA driver

Then start with the GPU override:

```bash
docker compose -f docker-compose.yml -f docker-compose.nvidia.yml up --build -d
```

The worker test-encodes at startup and logs `hardware encoder: nvenc`, or falls back to the CPU (and says why) if the GPU can't be used. Use the same two `-f` flags on later `up` commands, or it goes back to CPU encoding.

## Let other people join

Out of the box every URL points at `localhost`, which only works on the machine running Docker. For friends on your network (or the internet), browsers must be able to reach three things: the web app (3000), the API (8088) and the storage endpoint used for uploads (9044).

Edit `docker-compose.yml` and replace `localhost` with the address people will use (a LAN IP like `192.168.1.20`, or a domain):

| Service | Variable | Example |
| --- | --- | --- |
| `movie` | `ALLOWED_ORIGINS` | `http://192.168.1.20:3000` |
| `movie` | `S3_PUBLIC_ENDPOINT` | `http://192.168.1.20:9044` |
| `worker` | `S3_PUBLIC_ENDPOINT` | `http://192.168.1.20:9044` |
| `web` | `ORIGIN` | `http://192.168.1.20:3000` |
| `web` | `PUBLIC_API_URL` | `http://192.168.1.20:8088` |

Then `docker compose up -d` again (no rebuild needed) and share `http://192.168.1.20:3000`.

For people outside your network, forward ports 3000, 8088 and 9044 on your router, or use a VPN such as Tailscale (simplest: everyone joins your tailnet and uses the machine's Tailscale address).

> **Before exposing it beyond your own network**, change the secrets that ship in `docker-compose.yml`: `PLAYBACK_SECRET`, the Postgres password (`POSTGRES_PASSWORD` and every `DATABASE_URL`) and the RustFS keys (`RUSTFS_ACCESS_KEY` / `RUSTFS_SECRET_KEY` and every `S3_ACCESS_KEY` / `S3_SECRET_KEY`). Nicknames are not authenticated: anyone who can open the site can upload, delete and use any nickname, including a room host's. Only share it with people you trust.

## Using it

### Nickname

The first visit asks for a nickname (stored in your browser). It's shown to others in rooms and chat, and it decides who hosts a room. Change it any time from the name in the top-right corner.

### Library

**Adding movies**: click **Add movie**, or drop video files anywhere on the library page. New movies go into the folder you have open.

- **From device**: pick one or more files; they upload one after another. Progress shows on the card and in the *In progress* panel, and keeps going if you close the dialog (but not if you close the tab).
- **Torrent**: paste a magnet link or choose a `.torrent` file. Only the video files are downloaded (samples are skipped); each becomes its own movie, and the torrent can be cancelled from *In progress*. The server doesn't seed.
- **.m3u8 link**: paste an HLS playlist URL (optionally a title and a Referer header if the site needs one). The stream is copied without re-encoding.

After a file arrives, the worker transcodes it; the card shows `processing 42%` until it's `ready`. Files named like `Show.S01E02.mkv` get their season and episode filled in automatically.

**Organising**

- **Folders**: *New folder* creates one in the current folder, so you can nest a series → *Season 1*, *Season 2*. Click a folder to open it; the breadcrumb takes you back up. Deleting a folder moves its contents up a level, it never deletes movies.
- **Moving**: drag cards onto a folder card or a breadcrumb, or use *Edit* → *Folder*.
- **Search and filters**: search by name; filter by status and by *not watched yet / in progress / watched* (tracked per browser); search this folder, including subfolders, or the whole library; sort by newest, name or season/episode.
- **Bulk actions**: click **Select**, tick movies (or *All*), then move, re-process, delete originals or delete them from the bar at the bottom. Dragging a selected card moves the whole selection.

**Editing a movie** (the pencil on a card):

- Change the title, folder, season and episode.
- **Watch together** starts a room with it.
- **Re-process** transcodes it again from the original (useful after an update that adds features like seek previews).
- **Delete original** frees the space of the uploaded file; the movie still plays, but can't be downloaded or re-processed any more.
- While a movie is uploading, downloading or processing, the only action is **Cancel**: it stops the work and removes the movie and its files (cancelling a *re-process* keeps the previous version instead).
- **Delete** (click twice) removes the movie and everything stored for it.

The download icon on a card downloads the original file.

**Storage** (button in the library toolbar) shows the space used by originals and streams, per folder and per movie, with buttons to delete originals.

### Watching together

Press **▶** on a ready movie (or *Watch together* in its editor). That creates a room and opens it; send the room's address to the others. The **Rooms** page lists open rooms with who's watching. A room is removed about a minute after the last person leaves.

**The host** (whoever created the room) controls playback; everyone else follows automatically. If anyone's player is buffering, the room pauses for everyone and shows who it's waiting for.

**Player controls**

| Action | Mouse | Keyboard |
| --- | --- | --- |
| Play / pause | click the picture, or ▶ | <kbd>Space</kbd> or <kbd>K</kbd> |
| Back 5 s / forward 10 s | ⟲ / ⟳ buttons | <kbd>←</kbd> / <kbd>→</kbd> |
| Seek | drag the bar (hover shows a preview frame) | |
| Fullscreen | double-click the picture | <kbd>F</kbd> |
| Mute / volume | speaker button; scroll over the volume for 1 % steps | <kbd>M</kbd> |
| Speed | `1×` menu (host) | |
| Quality | `Auto` menu (only affects you) | |

Your position in each movie is remembered in your browser: the home page's **Continue watching** row lets you pick up where you stopped, and a host starting a room starts at their saved position. Volume and quality choices are remembered too.

**Room panel** (can be hidden with the button at the top right of the player):

- **Watching**: everyone in the room with their ping. People who can control playback also see a coloured dot per viewer: green = in sync, orange/red = drifting, hover for the exact drift and how much they have buffered.
- **Host controls**: let *everyone* control playback, or give control to specific people (🎮), or hand the whole room to someone else (👑).
- **Up next**: queue movies to play automatically when the current one ends. *Next episodes* queues the rest of the current folder in season/episode order; reorder or remove items, or skip with *Play next*. The movie picker at the top switches immediately.
- **Private room**: hides the room from the list. People join with the **invite link** shown in the panel, or with an optional **password**. *New link* makes old invite links stop working.
- **Chat**.

## Developing

### Requirements

- Go 1.27+
- Node.js with pnpm (the version in `frontend/package.json` → `packageManager`)
- `ffmpeg` and `ffprobe` on `PATH` (for the worker)
- Docker, for Postgres and RustFS during development

### Run the services locally

Start only the database and storage in Docker:

```bash
docker compose up -d db rustfs
```

**API and worker**: the Go binaries read configuration only from environment variables (there is no `.env` loader). Export the values from [`backend/.env.example`](backend/.env.example), then in two terminals:

```bash
cd backend && go run ./cmd/api
```

```bash
cd backend && go run ./cmd/worker
```

**Frontend**: create `frontend/.env` from [`frontend/.env.example`](frontend/.env.example) (`PUBLIC_API_URL=http://localhost:8088`), then:

```bash
cd frontend && pnpm install && pnpm dev
```

The dev server runs on http://localhost:5173 (already in the default `ALLOWED_ORIGINS`).

### Build

```bash
cd backend && go build ./...                 # API: ./cmd/api, worker: ./cmd/worker
cd frontend && pnpm build                    # production build in frontend/build (adapter-node)
docker compose build                         # all images
```

### Checks and tests

```bash
cd backend && go vet ./...
cd backend && TEST_DATABASE_URL="postgres://movieuser:<password>@localhost:5432/local?sslmode=disable" go test ./...
cd frontend && pnpm check && pnpm lint
```

The Go tests that need Postgres create and drop a throwaway database per test (the compose `db` works); without `TEST_DATABASE_URL` they are skipped.

End-to-end tests (Playwright) run against a running stack, including the worker, because they upload and transcode small fixture videos:

```bash
cd frontend
pnpm exec playwright install chromium   # once
pnpm test:e2e                           # E2E_BASE_URL / E2E_API_URL default to localhost:3000 / :8088
```

### Changing the database schema

The schema is defined in `frontend/src/lib/server/db/schema.ts`. After editing it, generate a migration (it is written to `backend/migrations` and embedded in the Go binaries, which apply it on startup):

```bash
cd frontend && DATABASE_URL=postgres://unused pnpm db:generate
```

## Configuration reference

`docker-compose.yml` sets every value directly; the root `.env` only feeds `DB_PORT` (host port for Postgres) and `HW_ENCODER`. When running the Go binaries yourself, set these environment variables:

| Variable | Used by | Default | Meaning |
| --- | --- | --- | --- |
| `DATABASE_URL` | API, worker | (required) | Postgres connection string |
| `ADDR` | API | `:8088` | Listen address |
| `ALLOWED_ORIGINS` | API | `http://localhost:5173` | Comma-separated browser origins allowed for CORS and WebSockets |
| `PLAYBACK_SECRET` | API | `change-me` | Signs playback URLs; set a long random value |
| `S3_ENDPOINT` | API, worker | `http://localhost:9000` | Storage endpoint the servers use |
| `S3_PUBLIC_ENDPOINT` | API, worker | `http://localhost:9000` | Storage endpoint browsers use for uploads and downloads |
| `S3_REGION`, `S3_BUCKET` | API, worker | `us-east-1`, `media` | Bucket settings (the bucket is created if missing) |
| `S3_ACCESS_KEY`, `S3_SECRET_KEY` | API, worker | | Storage credentials |
| `WORK_DIR` | worker | `/tmp/work` | Scratch space for transcodes and torrents (needs room for the largest file ×2) |
| `HW_ENCODER` | worker | `none` | `nvenc` for NVIDIA hardware encoding |
| `WORKER_ADDR` | worker | `:8089` | `/healthz` and Prometheus `/metrics` |
| `PUBLIC_API_URL` | web | `http://localhost:8088` | API address as seen from browsers |

## Troubleshooting

- **Uploads fail with "Missing ETag"**: the storage bucket's CORS rule is set by the API at startup from `ALLOWED_ORIGINS`. Make sure the site's origin is listed there, then restart the `movie` service.
- **Works on the host but not for friends**: some URL still says `localhost`; see [Let other people join](#let-other-people-join).
- **A movie is stuck in "processing"**: check `docker compose logs worker`. The worker processes one movie at a time, so others wait in line. *Cancel* in the movie's editor stops and removes it.
- **NVENC isn't used**: the worker log says why at startup (`hardware encoder unavailable…`). Check that you started with `-f docker-compose.nvidia.yml` and that `docker run --rm --gpus all ubuntu nvidia-smi` works.
- **Disk filling up**: see *Library → Storage*; delete originals you don't need to re-process, and remove old movies.
- **Worker health / metrics**: from inside the compose network, `docker compose exec movie wget -qO- http://worker:8089/metrics` (queue length is `whatever_jobs{status="queued"}`, transcode time `whatever_job_duration_seconds{type="process"}`).
