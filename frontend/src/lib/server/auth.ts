import { env } from '$env/dynamic/private';

const API_URL = env.BACKEND_URL ?? 'http://127.0.0.1:8088';

export async function apiLogin(email: string, password: string) {
	const res = await fetch(`${API_URL}/auth/login`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ email, password })
	});
	if (!res.ok) throw new Error('invalid credentials');
	return res.json() as Promise<AuthResult>;
}

export async function apiRegister(email: string, password: string, username: string) {
	const res = await fetch(`${API_URL}/auth/register`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ email, password, username })
	});
	const data = await res.json().catch(() => null);
	if (!res.ok) {
		throw new Error(data?.error ?? 'registration failed');
	}
	return data as AuthResult;
}

export async function apiRefresh(refreshToken: string) {
	const res = await fetch(`${API_URL}/auth/refresh`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ refreshToken })
	});
	if (!res.ok) return null;
	return res.json() as Promise<AuthResult>;
}

export async function apiLogout(refreshToken: string) {
	await fetch(`${API_URL}/auth/logout`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ refreshToken })
	});
}

export interface AuthResult {
	accessToken: string;
	refreshToken: string;
	user: { username: string; id: string; email: string; role: 'admin' | 'user' };
}
