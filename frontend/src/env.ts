import { defineEnvVars } from '@sveltejs/kit/env';

// Read at runtime (not inlined at build), so compose's value applies without a
// rebuild. The fallback keeps the Docker build working without a .env.
// DATABASE_URL is only used by drizzle-kit, which reads process.env itself.
export const variables = defineEnvVars({
	PUBLIC_API_URL: {
		public: true,
		description: 'Browser-reachable Go API base URL.',
		schema: (v) => v || 'http://localhost:8088'
	}
});
