import { nickname } from './nickname.svelte';
import { PUBLIC_API_URL } from '$app/env/public';

export async function apiFetch<T = void>(path: string, options: RequestInit = {}): Promise<T> {
	const headers = new Headers(options.headers);
	// Header values must be ASCII; the backend URI-decodes this.
	headers.set('X-Nickname', encodeURIComponent(nickname.name));

	if (!headers.has('Content-Type') && options.body) {
		headers.set('Content-Type', 'application/json');
	}

	const response = await fetch(`${PUBLIC_API_URL}${path}`, {
		...options,
		headers
	});

	if (response.status === 204) return undefined as T;

	if (!response.ok) {
		const err = await response.json().catch(() => ({ error: 'Unknown error' }));
		throw new Error(err.error || 'API Error');
	}

	return response.json();
}

export function apiPost<T = void>(path: string, body: unknown): Promise<T> {
	return apiFetch<T>(path, { method: 'POST', body: JSON.stringify(body) });
}
