import { jwtVerify } from 'jose';
import { AUTH_JWT_SECRET } from '$env/static/private';

const secret = new TextEncoder().encode(AUTH_JWT_SECRET);

export interface Claims {
	username: string;
	email: string;
	role: 'admin' | 'user';
	sub: string;
	exp: number;
}

function isClaims(payload: unknown): payload is Claims {
	if (typeof payload !== 'object' || payload === null) return false;
	const p = payload as Record<string, unknown>;
	return (
		typeof p.sub === 'string' &&
		typeof p.email === 'string' &&
		typeof p.username === 'string' &&
		(p.role === 'admin' || p.role === 'user')
	);
}

export async function verifyAccessToken(token: string): Promise<Claims | null> {
	try {
		const { payload } = await jwtVerify(token, secret);
		return isClaims(payload) ? payload : null;
	} catch (err) {
		console.error('verifyAccessToken failed:', err);
		return null;
	}
}
