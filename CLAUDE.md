# CLAUDE.md

Self-hosted watch-party app. Users upload videos (or add torrents); a worker transcodes them to HLS and stores them in S3, and people watch together in synced rooms. There are no accounts: users pick a nickname, which the frontend sends with every request.

## Layout

```
docker-compose.yml   full stack: rustfs (S3), postgres, movie (API), worker, web, proxy (HTTPS)
docker-compose.nvidia.yml  override: worker encodes with NVENC on an NVIDIA GPU
proxy/               nginx.conf + entrypoint.sh: TLS in front of web+API (1488) and S3 (9044), certs fetched from CERTS_URL
.env.example         compose variables (copy to .env)
backend/             Go module `liotom/whateverserver`
  Dockerfile         targets `api` (Alpine) and `worker` (Debian: its ffmpeg has h264_nvenc, Alpine's doesn't)
  cmd/api/           HTTP + WebSocket server (binary: movie-server)
  cmd/worker/        job runner for transcoding, torrents and imports, plus /healthz and /metrics (binary: movie-worker)
  internal/
    config/          env-var config (Config.Load)
    db/              pgxpool connect
    migrate/         applies the embedded migrations at startup (schema_migrations table, advisory lock)
    httpapi/         chi router and REST handlers (media + bulk, uploads, imports, torrents, rooms, playback, events)
    rooms/           in-memory watch-party hub, WebSocket protocol, queue/end timer, access, state persistence
    events/          change-notification Bus + Postgres LISTEN relay behind /api/events
    jobs/            Postgres-backed job queue (Enqueue / Claim / Done / Fail)
    transcode/       ffprobe → plan → ffmpeg HLS (fMP4, libx264 or NVENC), progress, subs/fonts, poster, preview sprites
    torrent/         anacrolix/torrent downloader that imports video files as media
    storage/         S3 client (multipart presign, get/put, delete prefix, sizes), content types, name helpers
    testdb/          throwaway migrated database per test (TEST_DATABASE_URL)
  migrations/        drizzle-kit SQL + embed.go (0000 schema, 0001 NOTIFY triggers, 0002 folders, 0003 rooms/media
                     features, 0004 row-level JSON event payloads)
frontend/            SvelteKit 3 + Svelte 5 (runes), plain CSS (no Tailwind), adapter-node, pnpm
  src/env.ts         declared env vars (DATABASE_URL, PUBLIC_API_URL)
  src/lib/
    api.ts           apiFetch<T>() / apiPost<T>(): prefix PUBLIC_API_URL, add the X-Nickname header
    events.ts        onEvents(topics, fn) / onChange(topics, fn): shared /api/events socket, batched typed events
    media.svelte.ts  MediaList: the media list kept current from events (progress patched, changed rows refetched)
    progress.ts      per-viewer watch positions and "watched" marks in localStorage, listProgress()
    folders.ts       folder paths, subtrees and select options
    episodes.ts      season/episode order (byEpisode) and nextEpisodes() for the room queue
    watch.ts         startRoom(item): create a room at the saved position and open it
    bytes.ts, time.ts  formatBytes(), formatTime()
    nickname.svelte.ts  `nickname` singleton, kept in localStorage
    room.svelte.ts   RoomSocket: WebSocket client, reconnect, server clock offset, target position, canControl,
                     queue; saveInvite()/savedInvite() for private rooms
    upload.svelte.ts `uploader` singleton: browser → S3 multipart upload with a FIFO queue, progress survives closing the modal
    types.ts         API response types and RoomIn/RoomOut WS messages (mirror the Go structs)
    components/      Modal (<dialog>), Player (hls.js, drift sync, sync status, buffer bar, seek previews, skip,
                     shortcuts, quality, resume), UploadModal (tabs: Uploader / Torrents / UrlImport),
                     MediaEditModal, FolderModal, Downloads
    server/db/       drizzle schema.ts: source of truth for the DB schema (used only by drizzle-kit)
  src/routes/        / (continue watching, rooms with live viewers), /library (search/filters, select + bulk,
                     file drop uploads, play-on-poster cards, add/edit modals), /library/storage (space per folder/movie),
                     /rooms/[id] (player, members + sync status, queue, chat, host settings)
  e2e/               Playwright specs (+ fixtures/: two 12s test episodes)
```

## How it fits together

- **Upload:** `POST /api/uploads/init` creates a `media_files` row (`uploading`) and an S3 multipart upload. For each 64 MB part the browser asks `POST /api/uploads/{id}/presign` for a URL just before PUTting it straight to S3 (so long uploads never hold expired URLs), then `POST /api/uploads/{id}/complete` finishes it, sets `processing` and enqueues a `process` job. A failed upload deletes its row; deleting media also aborts any unfinished multipart upload under `raw/{id}/`.
- **Torrent:** `POST /api/torrents` stores a magnet link or base64 metainfo and enqueues a `torrent` job. The worker downloads only the video files (skipping "sample"), uploads each one to `raw/{id}/…`, inserts `media_files` rows and enqueues a `process` job for each.
- **Link import:** `POST /api/imports` (`url`, optional `name`, `referer`) creates a `media_files` row (`downloading`) and enqueues an `import` job. The worker probes the playlist for its length, then remuxes the `.m3u8` stream into MKV with `ffmpeg -c copy` (network protocols only, via `-protocol_whitelist`), uploads it as the raw file and enqueues `process`. Master playlists use ffmpeg's default stream pick (best video plus one audio).
- **Process:** the worker downloads the raw file, then runs `ffprobe` → `BuildPlan` → `ffmpeg`. It copies H.264 8-bit video and AAC ≤2ch audio as is and re-encodes everything else. It writes `hls/{id}/master.m3u8`, segments, `subs/*.vtt|.ass`, `fonts/`, `thumbs/sprite_NNN.jpg` and `poster.jpg`, then marks the media `ready` with `raw_bytes`/`hls_bytes` and `thumbnails`. Video variants: the source quality (copied when browser-safe) plus up to two lower ladder steps (1080/720/480/360p, `variants()` in `transcode/hls.go`), named `stream_{height}p`. When the source is copied, the encoded ones use `-force_key_frames source` so all variants cut segments at the same times. With one audio track, audio is muxed into every video variant; with several, they are shared alternate renditions. `POST /api/media/{id}/reprocess` re-runs it from the stored original (old output is deleted first); media processed before the ladder or the preview sprites existed only gets them after a re-process.
- **Encoder:** `HW_ENCODER=nvenc` makes re-encoded variants use `h264_nvenc` (`Encoder.videoArgs` in `transcode/encoder.go`; CQ is CRF+4, which gave similar sizes). Scaling and pixel-format conversion stay on the CPU, so any source works. At startup `transcode.Detect` test-encodes a tiny clip and falls back to libx264 (logged) if the GPU isn't usable, and a job whose NVENC encode fails is retried once in software. In Docker this needs `docker-compose.nvidia.yml` (GPU reservation + `NVIDIA_DRIVER_CAPABILITIES=…video…`).
- **Progress:** imports and transcodes run ffmpeg with `-progress pipe:1` and store `media_files.progress` (0..1), at most every 3s and on ≥1% change, since each write makes open library pages refetch. It's null when idle or when the length is unknown (the UI then shows an indeterminate bar).
- **Preview sprites:** `transcode.Sprites` decodes keyframes only (`-skip_frame nokey`) and tiles 160×90 frames 10×10 per sheet, one every 2–10s (`thumbInterval`). The Player computes the sheet and offset from `thumbnails` on seek-bar hover.
- **Cancel:** `POST /api/media/{id}/cancel` stops an upload, download or transcode. Media that was never ready is deleted with everything stored for it. A re-process (`processing` with `hls_key` set) goes back to `ready` and keeps its previous output. The worker's `watchCancel` (`transcode/cancel.go`) polls the row every 2s and cancels the job's context when the row is gone or (for `Process`) back to `ready`, which kills ffmpeg. The job then returns `ErrCancelled` and is marked done, not failed. `Process` also skips a job whose row is no longer `processing`/`failed` (cancelled while queued). The final replace phase (delete old output, upload new, mark ready) runs uncancellable, so a cancel landing there keeps the new version, or deletes the output if the row is gone. In the browser that is uploading, `uploader.cancel()` aborts the part XHRs, and the upload's own cleanup deletes the row.
- **Bulk and storage:** `POST /api/media/bulk {ids, action: move|delete|reprocess|delete_original, folderId?}` returns `{done, skipped}` and skips media an action doesn't apply to. `POST /api/media/{id}/delete-original` (ready media only) deletes the raw file and sets `raw_deleted`; download and re-process then answer 409. Sizes for media processed before sizes were recorded are filled in by the worker's janitor (at startup, then hourly) from S3 listings. `/library/storage` sums them per folder subtree on the client.
- **Folders:** `folders` (id, name, parent_id) nest arbitrarily (a series → seasons → …); `media_files.folder_id` and `torrents.folder_id` point into them (null = top level; a torrent's videos inherit its folder). `/api/folders` lists/creates; `PATCH` renames or moves (refuses cycles via a recursive CTE); `DELETE` moves the folder's subfolders, media and torrents up to its parent and never deletes content. Uploads, imports and torrents accept `folderId`; `PATCH /api/media/{id}` moves media (`folderId: null` = top level). The library shows one folder at a time via `?folder=<id>`, and the frontend builds paths and subtrees itself (`src/lib/folders.ts`).
- **Download:** `GET /api/media/{id}/download` 302-redirects to a 15-minute presigned S3 link for the original (`raw_key`) with an attachment filename.
- **Live updates:** row triggers (migration 0004) `pg_notify('events', <json>)` saying what changed:
  - `{"type":"media_files"|"torrents"|"rooms","op":"INSERT|UPDATE|DELETE","id":…}`
  - `{"type":"media_progress","id","progress"}` and `{"type":"torrent_progress","id","progress","bytesDone","bytesTotal"}` when only progress columns changed
  - `{"type":"folders"}` (statement level)

  Updates that change nothing, and `rooms` state flushes/touches, stay silent. The API LISTENs on its own connection and forwards the payloads on the `/api/events` WebSocket. The hub sends `{"type":"rooms","id":…}` itself on join/leave/playback changes. A subscriber that falls behind is disconnected (`events.Bus`), and the browser resyncs on reconnect. Frontend: `onEvents(topics, (events, resync) => …)` gets debounced batches, and `onChange` ignores the details. Media lists use `MediaList` (`src/lib/media.svelte.ts`): it patches progress, refetches only the changed rows (`GET /api/media/{id}`, or the whole list above 20 in one burst) and drops deleted ones. Pages never poll. `GET /api/rooms` merges the hub's live members and state.
- **Playback:** media responses include `playbackUrl` = `/play/{hmacToken}/{id}`, signed with `PLAYBACK_SECRET`. The expiry is rounded up to a 6h step (valid 12–18h), so the URL is identical across API calls within a window; posters stay cached and nothing keyed on it remounts. The client appends `/master.m3u8`, and the API proxies the files from S3 with Range support.
- **Rooms:** the WebSocket is at `/api/rooms/{id}/ws?nickname=…[&invite=…]`. Client → server: `play|pause|seek|rate|change_media|queue|next|buffering|status|chat|ping`. Server → client: `state` (playback, incl. the queue), `queue` (only the queue changed), `room` (`RoomInfo`: name, owner, everyone, controllers, private, hasPassword, and `inviteCode` for the owner only; sent on join and after every settings `PATCH`), `members`, `chat`, `pong` and `error` (see `rooms/messages.go`). The room page reads its settings from `room` messages. It calls `GET /api/rooms/{id}` only when the socket was refused (4003), for the name and password prompt; the owner gets the invite code that way and reconnects. The server is authoritative: position = `position + (now - updatedAt) * rate` while playing. If any member is buffering, the room pauses and sets `holding`. Playback is controlled by the owner (matched by nickname), the nicknames in `controllers`, or everyone when `everyone_controls` is on; `PATCH /api/rooms/{id}` (owner only) sets these and `ownerName` hands the room over. `change_media` and `queue` IDs are checked against ready media through `Room.lookup` (called before `Room.mu` is taken).
- **Queue:** `state.queue` holds media IDs (`queue` replaces the whole list, `next` skips to its head). The room keeps each media's `duration_sec` and arms an end timer (`scheduleEndLocked`, re-armed on every state broadcast). At the end it plays the head of the queue from 0, or pauses at the end when the queue is empty. Clients don't report `ended`. "Next episodes" in the UI queues the later episodes of the current folder in season/episode order.
- **Sync status:** every 2s each Player sends `status {drift, ahead}` (its offset from the room clock and seconds buffered). Each 5s `ping` also carries the previous round trip (`rtt`, ms), stored as the member's `ping` when it moves by 5 ms or more. The server sends `members` (with `drift`/`ahead`/`ping`) at most every 2s (`Hub.Run`'s members tick), never once per message. Everyone sees each viewer's ping; controllers also get a coloured drift dot whose tooltip shows drift and buffer.
- **Private rooms:** `private` rooms aren't listed (except to their owner) and need `invite_code`. `GET /api/rooms/{id}` without a valid `X-Room-Invite` header (and not the owner) returns only `{id, name, private, locked, hasPassword}`. `POST /api/rooms/{id}/access {password}` (bcrypt) returns the invite code, so the password never goes in a URL. The WebSocket closes with **4003** when the invite is missing or wrong; the frontend then shows the password form instead of treating the room as gone. Invite codes are kept per room in localStorage (`room-invite:{id}`); `resetInvite` makes old links stop working. The hub writes dirty state to `rooms.state` every 10s and evicts empty rooms (touching `updated_at`). `Hub.reap` then deletes rooms that aren't loaded and haven't been touched for 60s, so a room disappears about a minute after the last viewer leaves (or if its creator never arrives). Rooms are temporary; nothing should rely on them lasting.
- **Per-viewer state (localStorage only, `src/lib/progress.ts` and `Player.svelte`):** each viewer's position per media (`progress:{mediaId}`, saved every 5s while playing, on pause and on leave, and cleared near the start or end; reaching the end also sets `watched:{mediaId}`, used by the library's "watched" filter; the home page's Continue watching row is `listProgress()`), quality (`quality`, a height or `auto`, applied through hls.js levels), and volume/mute (`sound`). When a host starts a room from the library or switches media, the room starts at the host's saved position (`position` on `POST /api/rooms` and on `change_media`).
- **Jobs:** the `jobs` table is claimed with `FOR UPDATE SKIP LOCKED`. A job gets 3 attempts with a 30s backoff. On startup the worker requeues `running` jobs, which assumes a **single worker instance**. Its janitor (at startup, then hourly) deletes uploads stuck in `uploading` for a day (with their S3 parts) and `done`/`failed` jobs older than a week, and measures unmeasured media. Jobs for media deleted in the meantime end quietly; a process that finishes after its media was deleted removes its output.
- **Worker health and metrics:** the worker serves `WORKER_ADDR` (default `:8089`, exposed only inside the compose network): `/healthz` (200 while a DB ping succeeds; compose runs `movie-worker healthcheck`, since the Debian image has no curl/wget) and Prometheus `/metrics`: `whatever_jobs{type,status}` (queried at scrape time; `status="queued"` is the queue length), `whatever_job_duration_seconds{type,result}` (type `process` = transcode time), `whatever_jobs_running{type}`, `whatever_hw_encoder{encoder}`.
- **Migrations:** both binaries run `migrate.Up` at startup. It applies the embedded `backend/migrations` in `_journal.json` order, one transaction per file, recorded in `schema_migrations`, under a Postgres advisory lock so the API and worker don't race. A database without `schema_migrations` (made by the old initdb mount or `db:push`) is baselined: 0000/0002 count as applied when their tables exist, and the idempotent 0001 runs again.

## Running

### Full stack (Docker)

```bash
cp .env.example .env
docker compose up --build
```

With GPU transcoding (NVIDIA Container Toolkit; on Windows, Docker Desktop with WSL 2):

```bash
docker compose -f docker-compose.yml -f docker-compose.nvidia.yml up --build
```

Ports: the `proxy` (nginx) serves `https://$PUBLIC_HOST:1488` (web, with `/api` and `/play` routed to the API, so `PUBLIC_API_URL` is the same origin) and S3 at `https://$PUBLIC_HOST:9044` (`S3_PUBLIC_ENDPOINT`; presigned URLs sign the Host, so nginx forwards `$http_host` with its port). It downloads `fullchain.pem`/`privkey.pem` from `CERTS_URL` at start and every `CERTS_REFRESH` seconds, reloading on change. Also published: API 8088 (plain HTTP), RustFS console 9045, Postgres 5432. The worker's 8089 isn't published.

The schema is created and upgraded by the binaries at startup (see Migrations), on fresh and existing volumes alike. To change it: edit `schema.ts`, run `pnpm db:generate` (writes the next `backend/migrations/NNNN_*.sql` and the journal; it needs `DATABASE_URL` set but doesn't connect), append hand-written SQL such as triggers to that file, and restart. Don't edit a migration that has already been applied anywhere; add a new one. A new table that pages should follow needs its own `*_notify` trigger appended to its migration by hand. `pnpm db:push` is only for throwaway dev databases: it bypasses `schema_migrations`, so the runner would later try to re-apply those changes.

### Backend locally

Requires Go 1.27+, and `ffmpeg`/`ffprobe` on PATH for the worker. Config is read **only from the process environment** (no .env loader), so export the values from `backend/.env.example` first. Run postgres and rustfs with `docker compose up db rustfs`.

```bash
cd backend && go run ./cmd/api
```

```bash
cd backend && go run ./cmd/worker
```

Checks: `go build ./...`, `go vet ./...`, `go test ./...`. Both binaries exit if `DATABASE_URL` is unset; the API warns when `PLAYBACK_SECRET` is left at its insecure default. `ADDR` defaults to `:8088`, `WORKER_ADDR` to `:8089`, `HW_ENCODER` to `none`.

Tests: `jobs`, `rooms` (hub), `httpapi` and `migrate` need Postgres. They create and drop a database per test (`internal/testdb`), so point `TEST_DATABASE_URL` at a server where that user may `CREATE DATABASE` (the compose db works); without it they're skipped. The room logic tests (`rooms/room_test.go`, with a fake `nowMs`) and `transcode` tests are pure. `-race` needs cgo; on Windows run it in a container (`golang:1.27.2`, DB at `host.docker.internal`).

```bash
cd backend && TEST_DATABASE_URL=postgres://movieuser:<pw>@localhost:5432/local?sslmode=disable go test ./...
```

### Frontend locally

pnpm with `engine-strict`. Set `DATABASE_URL` and `PUBLIC_API_URL` (e.g. `http://localhost:8088`) in `frontend/.env`.

```bash
cd frontend && pnpm install && pnpm dev
```

Other scripts: `pnpm build`, `pnpm check` (svelte-check), `pnpm lint` (prettier + eslint), `pnpm format`, `pnpm test:e2e` (Playwright), and `pnpm db:generate|db:push|db:studio` (drizzle-kit; `db:migrate` would bypass `schema_migrations`, don't use it).

E2E: `pnpm exec playwright install chromium` once. The tests run against a running stack (worker included: they upload and transcode the fixtures) at `E2E_BASE_URL` (default `http://localhost:3000`) and `E2E_API_URL` (default `http://localhost:8088`). They name everything after a per-run ID and delete what they created. Specs: library (nickname, folder, upload, search, bulk move, file drop, continue watching, storage/delete original), room (sync, chat, sync status, queue, control handover), private rooms.

## Conventions

### Go
- Use raw SQL through `pgxpool`, with no ORM. Column names are snake_case and must match `frontend/src/lib/server/db/schema.ts`.
- Handlers are methods on `httpapi.Server`. Respond with `writeJSON` / `writeErr` (`{"error": "..."}`); mutations without a body return `204`.
- JSON fields are camelCase. Optional columns are pointer fields with `omitempty`. JSONB columns use `json.RawMessage` in API structs.
- PATCH handlers take pointer fields and use `COALESCE($n, col)`. When "omitted" and "null" must differ, use `optInt` (see `PATCH /api/media/{id}`, which also re-parses season/episode from a new filename).
- IDs are `uuid.NewString()` stored as `text`. The only exception is `jobs.id`, which is a bigserial.
- The caller's identity is `nickname(r)`, which URI-decodes the `X-Nickname` header and passes it through `rooms.CleanNickname` (trimmed, at most 32 runes, default "Anonymous"). Nothing is authenticated. Room settings and deletion check the nickname against `owner_name` (`requireRoomOwner`).
- When a handler changes a room that may be loaded in the hub, update the hub too (`Hub.SetAccess` with the row's owner/everyone/controllers/private/invite, `Hub.Remove`). The hub otherwise keeps its own copy until the room is evicted.
- Never return `password_hash` or `invite_code` in API structs; `roomRow` keeps them unexported, and only the owner's `GET /api/rooms/{id}` adds `inviteCode`.
- Batch writes into one statement where you can (`id = ANY($1)`, `unnest`). Triggers notify per row, and the browser batches a burst into one handler call (one list reload past 20 rows).
- Don't rewrite a column with its current value just to "touch" a row that pages follow: unchanged updates are silent, but progress-only updates must stay progress-only to reach pages as `*_progress` events.
- Status strings: media `uploading|downloading|processing|ready|failed`, torrent `queued|downloading|importing|done|failed|cancelled`, job `queued|running|done|failed`.
- S3 key layout: `raw/{mediaId}/{SafeName(filename)}` for originals and `hls/{mediaId}/…` for transcoded output.
- Long-running work belongs in the worker as a job type, not in a request handler.
- Lock in `rooms`: methods ending in `Locked` expect `Room.mu` to be held.

### Frontend
- Use Svelte 5 runes only (runes mode is forced in `vite.config.ts`). Shared reactive state lives in `*.svelte.ts` classes.
- Import from `#lib/...` with explicit `.ts` extensions (e.g. `#lib/api.ts`). Read env through `$app/env/public` / `$app/env/private`, declared in `src/env.ts`.
- Make all backend calls through `apiFetch` so the nickname header is sent. Build internal links with `resolve()` from `$app/paths`.
- When a Go JSON struct changes, update `src/lib/types.ts`. When a table changes, update `src/lib/server/db/schema.ts` (the Go code does not define the schema).
- Plain CSS only, no Tailwind. `src/routes/app.css` holds the fielv-store look: tokens (`--color-primary|secondary|bg|text|accent|border|muted|gray|warn|ok`, `--radius`, `--header-height`), base styles, and shared classes (`container`, `page`, `page-head`, `panel`, `section-title`, `panel-title`, `muted`, `meta`, `error-text`, `badge`, `status-<media status>`, `field`, `label`, `input`, `btn btn-primary|btn-outline|btn-danger`, `btn-icon`, `btn-round`, `progress`, `spinner`). Layout specific to a page or component goes in its scoped `<style>` block, using the tokens rather than raw colors. Fonts are served from `static/fonts`; icons come from `@lucide/svelte`.
- Don't poll, and don't refetch whole lists on every event. Use `MediaList` for media. For other data, use `onEvents` to patch what the event names, or `onChange([...topics], load)` for small lists (folders, torrents). Return the unsubscribe from `onMount`/`$effect`. Room state and settings come over the room socket, not REST.
- Modals use `Modal.svelte` (a native `<dialog>`). It focuses the first form field after `showModal()`, so don't autofocus inside modal content yourself (`showModal()` would override it).
- Browser uploads PUT parts with XHR (not fetch) because only XHR reports upload progress; the `uploader` singleton exposes `progress`, `mediaId`, `size` and `queue` for the card overlay and the In progress list. Use `uploader.enqueue(files, folderId)`; `upload()` throws while another upload runs.
- Hover highlight on cards is the `--ring` token (solid outline); library cards have none (the ring there marks selected cards).
- Library drag and drop is for uploads only: files dragged in from the desktop are recognised by the `Files` type and go to `uploader.enqueue`. Cards aren't draggable; movies move through Select (bulk) or the edit modal.
- On a ready movie card, the whole poster is a play button (`.play`, its round icon shows on hover, always visible on touch screens) that starts or resumes a room via `startRoom`. There is no separate play button in the card footer.
- Start rooms with `startRoom()` from `#lib/watch.ts` (saves the invite code of private rooms).
- Prettier settings: tabs, single quotes, no trailing commas, width 100.

## Known gaps / gotchas

- `PUBLIC_API_URL` is read at runtime (not inlined at build), so it can be changed in compose without rebuilding. The `web` service doesn't read `DATABASE_URL` or `BACKEND_URL`; compose still passes them.
- Browser multipart upload needs the bucket CORS to expose `ETag`. Only the API sets it (`Storage.SetCORS` from `ALLOWED_ORIGINS`, on startup, logging if the S3 server rejects it). Don't call it from the worker: it has no origins configured and would overwrite the rule.
- ASS subtitles and MKV fonts are extracted (`subs/*.ass`, `fonts/`) but the player only shows the WebVTT conversion; styled ASS rendering isn't implemented.
- `storage.UploadFile` uses `feature/s3/manager`, which is deprecated in favour of `transfermanager`; it still works.
- Link imports fetch any http(s) URL from the worker, including private network addresses.
- The room WebSocket is closed with code 1008 when the room doesn't exist or is deleted. The frontend treats that as final and doesn't reconnect. 4003 means "invite required" (private room).
- Nicknames aren't authenticated, so ownership, control and the owner's view of a private room's invite code all trust whoever sends that nickname. Private rooms keep strangers out of the listing and the socket, nothing more.
- The room's end timer uses `duration_sec` from ffprobe, which can be a little longer than the HLS stream; the player then sits at the end for a moment before the queue moves on.
- Alpine's ffmpeg has no NVENC, hence the Debian worker image. VAAPI/QSV aren't wired up (`ParseEncoder` only knows `nvenc` and `none`).
- Existing media get preview sprites only after a re-process. Sizes are backfilled automatically.
- ffmpeg only expands `%v` in `-hls_fmp4_init_filename` when there are several variants; single-variant output needs a literal init name (see `RunHLS`).
- Player mouse: a click on the picture toggles play/pause for controllers after `DOUBLE_CLICK_MS` (220 ms), and a double-click cancels it and toggles fullscreen.
- Volume: the wheel over the volume controls changes it 1% per 50px of scroll (`soundEl` listener, non-passive so the page doesn't scroll); the slider steps 1% too.
- Playback drift: the player avoids changing `playbackRate` repeatedly (Chrome's audio falls behind the video when it does). It hard-seeks past 1s, uses one fixed ±5% step between 0.25s and 1s, and resets under 0.05s.
- `S3_ENDPOINT` is used by the server for its own S3 calls. `S3_PUBLIC_ENDPOINT` must be reachable from the browser because presigned upload URLs are signed against it.
