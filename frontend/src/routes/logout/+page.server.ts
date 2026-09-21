import { redirect, type Actions } from '@sveltejs/kit';
import { apiLogout } from '$lib/server/auth';

export const actions: Actions = {
	default: async ({ cookies }) => {
		const refreshToken = cookies.get('refresh_token');
		if (refreshToken) await apiLogout(refreshToken);
		cookies.delete('access_token', { path: '/' });
		cookies.delete('refresh_token', { path: '/' });
		throw redirect(303, '/');
	}
};
