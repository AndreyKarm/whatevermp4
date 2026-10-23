import { apiFetch } from './api';
import { onEvents, type ServerEvent } from './events';
import type { MediaItem } from './types';

// Above this many changed rows in one burst, reloading the list is cheaper
// than fetching each one.
const MAX_SINGLE_FETCHES = 20;

/**
 * The media library, loaded once and then kept current from server events:
 * progress updates are patched in, changed rows refetched one by one and
 * deleted ones dropped, instead of reloading the whole list each time.
 */
export class MediaList {
	items = $state<MediaItem[]>([]);
	loaded = $state(false);
	error = $state<string | null>(null);

	async load() {
		try {
			this.items = await apiFetch<MediaItem[]>('/api/media');
			this.error = null;
		} catch (e) {
			this.error = e instanceof Error ? e.message : String(e);
		} finally {
			this.loaded = true;
		}
	}

	/** Loads and follows changes; returns the unsubscribe for onMount/$effect. */
	live(): () => void {
		this.load();
		return onEvents(['media_files', 'media_progress'], (events, resync) => {
			if (resync) this.load();
			else this.#apply(events);
		});
	}

	async #apply(events: ServerEvent[]) {
		// the last word per row wins: id → deleted?
		const changed: Record<string, boolean> = {};
		for (const e of events) {
			if (e.type === 'media_progress') {
				const m = this.items.find((m) => m.id === e.id);
				if (m) m.progress = e.progress ?? undefined;
			} else if (e.type === 'media_files') {
				changed[e.id] = e.op === 'DELETE';
			}
		}
		const n = Object.keys(changed).length;
		if (!n) return;
		if (n > MAX_SINGLE_FETCHES) return this.load();

		const ids = Object.keys(changed).filter((id) => !changed[id]);
		const fresh = await Promise.all(
			ids.map((id) => apiFetch<MediaItem>(`/api/media/${id}`).catch(() => null))
		);
		const byId: Record<string, MediaItem> = {};
		for (const item of fresh) if (item) byId[item.id] = item; // null: gone in the meantime
		// replace changed rows in place, drop deleted ones, put new ones first (the API's order)
		const known = this.items.map((m) => m.id);
		const added = Object.values(byId).filter((m) => !known.includes(m.id));
		const kept = this.items
			.map((m) => (m.id in changed ? byId[m.id] : m))
			.filter((m): m is MediaItem => !!m);
		this.items = [...added, ...kept];
	}
}
