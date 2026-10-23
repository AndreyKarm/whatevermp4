import type { MediaItem } from './types';

export const byFilename = (a: MediaItem, b: MediaItem) =>
	a.filename.localeCompare(b.filename, undefined, { numeric: true, sensitivity: 'base' });

/** Season/episode order: numbered items first (S1E1, S1E2, …, S2E1), the rest by name. */
export function byEpisode(a: MediaItem, b: MediaItem) {
	const key = (m: MediaItem) => (m.season ?? Infinity) * 10_000 + (m.episode ?? 0);
	return key(a) - key(b) || byFilename(a, b);
}

/**
 * The ready media that come after `current` in its folder, in season/episode
 * order: what "queue next episodes" lines up.
 */
export function nextEpisodes(library: MediaItem[], current: MediaItem): MediaItem[] {
	const folder = current.folderId ?? null;
	const list = library
		.filter((m) => m.status === 'ready' && (m.folderId ?? null) === folder)
		.sort(byEpisode);
	const i = list.findIndex((m) => m.id === current.id);
	return i < 0 ? [] : list.slice(i + 1);
}
