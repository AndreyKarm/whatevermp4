import { fail, redirect } from '@sveltejs/kit';
import type { Actions } from './$types';
import { apiRegister } from '$lib/server/auth';

const COOKIE_OPTS = { httpOnly: true, sameSite: 'lax' as const, path: '/', secure: false };

export const actions: Actions = {
	default: async ({ request, cookies }) => {
		const form = await request.formData();
		const email = (form.get('email') as string)?.trim();
		const password = form.get('password') as string;
		const username = (form.get('username') as string)?.trim();

		if (!email || !password || !username) {
			return fail(400, { error: 'Email, password and Username are required' });
		}
		if (username.length < 2 || username.length > 32) {
			return fail(400, { error: 'Username must be 2-32 characters' });
		}

		try {
			const result = await apiRegister(email, password, username);
			cookies.set('access_token', result.accessToken, { ...COOKIE_OPTS, maxAge: 60 * 15 });
			cookies.set('refresh_token', result.refreshToken, {
				...COOKIE_OPTS,
				maxAge: 60 * 60 * 24 * 30
			});
		} catch (err) {
			return fail(409, {
				error: err instanceof Error ? err.message : 'Error registering user'
			});
		}

		throw redirect(303, '/');
	}
};
