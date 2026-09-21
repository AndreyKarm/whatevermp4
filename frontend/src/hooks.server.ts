import type { Handle } from '@sveltejs/kit';
import { verifyAccessToken } from '$lib/server/jwt';
import { apiRefresh } from '$lib/server/auth';

const COOKIE_OPTS = { httpOnly: true, sameSite: 'lax' as const, path: '/', secure: false };
// set secure: true once you're behind HTTPS

export const handle: Handle = async ({ event, resolve }) => {
	let accessToken = event.cookies.get('access_token') ?? null;
	const refreshToken = event.cookies.get('refresh_token');

	let claims = accessToken ? await verifyAccessToken(accessToken) : null;

	if (!claims && refreshToken) {
		const result = await apiRefresh(refreshToken);
		if (result) {
			accessToken = result.accessToken;
			event.cookies.set('access_token', result.accessToken, {
				...COOKIE_OPTS,
				maxAge: 60 * 15 // 15 minutes
			});
			event.cookies.set('refresh_token', result.refreshToken, {
				...COOKIE_OPTS,
				maxAge: 60 * 60 * 24 * 30 // 30 days
			});
			claims = await verifyAccessToken(accessToken);
		} else {
			event.cookies.delete('access_token', { path: '/' });
			event.cookies.delete('refresh_token', { path: '/' });
			accessToken = null; // don't leak the stale/invalid token into locals
		}
	}

	event.locals.user = claims
		? { username: claims.username, id: claims.sub, email: claims.email, role: claims.role }
		: null;
	event.locals.accessToken = claims ? accessToken : null;

	return resolve(event);
};
