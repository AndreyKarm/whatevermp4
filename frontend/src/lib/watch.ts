import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { apiPost } from './api';
import { getProgress } from './progress';
import { saveInvite } from './room.svelte';
import type { MediaItem } from './types';

/** Starts a room for `item` at this viewer's saved position and opens it. */
export async function startRoom(item: MediaItem, opts: { private?: boolean } = {}) {
	const { id, inviteCode } = await apiPost<{ id: string; inviteCode?: string }>('/api/rooms', {
		name: item.filename,
		mediaId: item.id,
		// the room starts where this host last stopped
		position: getProgress(item.id) ?? 0,
		private: opts.private ?? false
	});
	if (inviteCode) saveInvite(id, inviteCode);
	await goto(resolve('/rooms/[id]', { id }));
}
