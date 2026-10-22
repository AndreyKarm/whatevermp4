# whatevermp4 frontend

SvelteKit 3 / Svelte 5 (runes) app for the watch-party server, styled with plain CSS.
It talks to the Go API in `../backend` over REST and WebSockets; see `../CLAUDE.md`
for the architecture, conventions and how to run the whole stack.

## Develop

```sh
pnpm install
pnpm dev
```

Set `PUBLIC_API_URL` (default `http://localhost:8088`) in `.env` if the API runs elsewhere.
`DATABASE_URL` is only needed for the drizzle-kit scripts below.

## Scripts

- `pnpm build`: production build (adapter-node, output in `build/`)
- `pnpm check`: svelte-check
- `pnpm lint`, `pnpm format`: prettier + eslint
- `pnpm db:push`, `pnpm db:generate`: apply / generate the schema in
  `src/lib/server/db/schema.ts` (migrations are written to `../backend/migrations`)
