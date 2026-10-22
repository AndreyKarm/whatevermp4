import { defineEnvVars } from '@sveltejs/kit/env';

// Variables without a schema are required at build time too, which breaks the Docker build
// (no .env in the image). Both are read at runtime, so compose's values still apply.
export const variables = defineEnvVars({
	DATABASE_URL: {
		description: 'The database connection string (only used by drizzle-kit and server/db).',
		schema: (v) => v || undefined
	},
	PUBLIC_API_URL: {
		public: true,
		description: 'Browser-reachable Go API base URL.',
		schema: (v) => v || 'http://localhost:8088'
	}
});
