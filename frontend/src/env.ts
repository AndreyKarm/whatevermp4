import { defineEnvVars } from '@sveltejs/kit/env';

export const variables = defineEnvVars({
	DATABASE_URL: { description: 'The database connection string.' },
	PUBLIC_API_URL: { public: true, description: 'Backend API URL.' }
});
