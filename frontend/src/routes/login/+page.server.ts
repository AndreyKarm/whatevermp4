import { fail, redirect } from '@sveltejs/kit';
import type { Actions } from './$types';
import { apiLogin } from '$lib/server/auth';

const COOKIE_OPTS = { httpOnly: true, sameSite: 'lax' as const, path: '/', secure: false };

export const actions: Actions = {
	default: async ({ request, cookies }) => {
		const form = await request.formData();
		const email = form.get('email') as string;
		const password = form.get('password') as string;

		try {
			const result = await apiLogin(email, password);
			cookies.set('access_token', result.accessToken, {
				...COOKIE_OPTS,
				maxAge: 60 * 15 // 15 minutes
			});
			cookies.set('refresh_token', result.refreshToken, {
				...COOKIE_OPTS,
				maxAge: 60 * 60 * 24 * 30 // 30 days
			});
		} catch {
			return fail(401, { error: 'Invalid email or password' });
		}

		throw redirect(303, '/');
	}
};
