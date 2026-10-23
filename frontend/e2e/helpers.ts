import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { expect, type Browser, type BrowserContext, type Page } from '@playwright/test';

export const API = process.env.E2E_API_URL ?? 'http://localhost:8088';
export const FIXTURES = path.join(import.meta.dirname, 'fixtures');
export const EP1 = path.join(FIXTURES, 'Test.Show.S01E01.mp4');
export const EP2 = path.join(FIXTURES, 'Test.Show.S01E02.mp4');

/** A name unique to this run, so leftovers of earlier runs never match. */
export const runId = `e2e-${Date.now().toString(36)}`;

export async function api<T = unknown>(
	nick: string,
	method: string,
	route: string,
	body?: unknown,
	headers: Record<string, string> = {}
): Promise<T> {
	const res = await fetch(API + route, {
		method,
		headers: {
			'Content-Type': 'application/json',
			'X-Nickname': encodeURIComponent(nick),
			...headers
		},
		body: body === undefined ? undefined : JSON.stringify(body)
	});
	if (!res.ok) throw new Error(`${method} ${route}: ${res.status} ${await res.text()}`);
	return (res.status === 204 || res.status === 202 ? undefined : await res.json()) as T;
}

/** Uploads a file the way the browser does (multipart, straight to S3); returns the media ID. */
export async function uploadViaApi(nick: string, file: string, folderId: string | null = null) {
	const data = await readFile(file);
	const { mediaId, uploadId } = await api<{ mediaId: string; uploadId: string }>(
		nick,
		'POST',
		'/api/uploads/init',
		{ filename: path.basename(file), contentType: 'video/mp4', folderId }
	);
	const { url } = await api<{ url: string }>(nick, 'POST', `/api/uploads/${mediaId}/presign`, {
		uploadId,
		partNumber: 1
	});
	const put = await fetch(url, { method: 'PUT', body: data });
	if (!put.ok) throw new Error(`PUT part: ${put.status}`);
	await api(nick, 'POST', `/api/uploads/${mediaId}/complete`, {
		uploadId,
		parts: [{ partNumber: 1, etag: put.headers.get('etag') }]
	});
	return mediaId;
}

/** Waits until the worker has transcoded the media. */
export async function waitReady(mediaId: string, timeout = 150_000) {
	await expect
		.poll(
			async () => (await api<{ status: string }>('e2e', 'GET', `/api/media/${mediaId}`)).status,
			{
				timeout,
				intervals: [1000]
			}
		)
		.toBe('ready');
}

/** A browser context whose localStorage already holds `nick` (skips the nickname prompt). */
export async function userContext(browser: Browser, nick: string): Promise<BrowserContext> {
	const ctx = await browser.newContext();
	await ctx.addInitScript((n) => localStorage.setItem('nickname', n), nick);
	return ctx;
}

export async function userPage(browser: Browser, nick: string): Promise<Page> {
	return (await userContext(browser, nick)).newPage();
}
