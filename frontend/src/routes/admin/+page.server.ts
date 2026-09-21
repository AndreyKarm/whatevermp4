import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = ({ locals }) => {
	if (!locals.user) throw error(401, 'Unauthenticated');
	if (locals.user.role !== 'admin') throw error(403, 'Forbidden');
	return { user: locals.user };
};
