import { defineConfig, devices } from '@playwright/test';

// End-to-end tests against a running stack (`docker compose up`, or the dev
// servers): the web app at E2E_BASE_URL and the API at E2E_API_URL. They
// upload small fixture videos, so the worker must be running too.
export default defineConfig({
	testDir: './e2e',
	// one shared backend: run the files one after another
	workers: 1,
	fullyParallel: false,
	timeout: 180_000,
	expect: { timeout: 15_000 },
	reporter: [['list'], ['html', { open: 'never' }]],
	use: {
		baseURL: process.env.E2E_BASE_URL ?? 'http://localhost:3000',
		trace: 'retain-on-failure',
		// let the players start without a click
		launchOptions: { args: ['--autoplay-policy=no-user-gesture-required'] }
	},
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }]
});
