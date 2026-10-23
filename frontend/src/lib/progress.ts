// Per-viewer watch positions, kept only in this browser's localStorage.
// Storage can be unavailable (private mode, blocked site data); everything
// here then quietly does nothing.

const PREFIX = 'progress:';
// Set when a movie was watched to (nearly) the end.
const WATCHED = 'watched:';

// Positions this close to the start or end don't count as "in progress".
const MIN_START = 10;
const END_MARGIN = 60;

interface Entry {
	t: number;
	at: number;
}

export interface Progress {
	mediaId: string;
	/** position in seconds */
	t: number;
	/** when it was saved (ms) */
	at: number;
}

/** Last saved position in seconds for this media, or null. */
export function getProgress(mediaId: string): number | null {
	try {
		const raw = localStorage.getItem(PREFIX + mediaId);
		if (!raw) return null;
		const e = JSON.parse(raw) as Entry;
		return typeof e.t === 'number' && e.t > 0 ? e.t : null;
	} catch {
		return null;
	}
}

/** Every saved position, most recent first. */
export function listProgress(): Progress[] {
	const out: Progress[] = [];
	try {
		for (let i = 0; i < localStorage.length; i++) {
			const key = localStorage.key(i);
			if (!key?.startsWith(PREFIX)) continue;
			const e = JSON.parse(localStorage.getItem(key) ?? 'null') as Entry | null;
			if (e && typeof e.t === 'number' && e.t > 0) {
				out.push({ mediaId: key.slice(PREFIX.length), t: e.t, at: e.at ?? 0 });
			}
		}
	} catch {
		/* storage unavailable */
	}
	return out.sort((a, b) => b.at - a.at);
}

/** Whether this viewer has watched the media to the end. */
export function isWatched(mediaId: string): boolean {
	try {
		return localStorage.getItem(WATCHED + mediaId) !== null;
	} catch {
		return false;
	}
}

/**
 * Saves the position. Near the start it clears it instead (not started);
 * near the end it clears it and marks the media watched.
 */
export function saveProgress(mediaId: string, t: number, duration: number) {
	try {
		if (duration > 0 && duration - t < END_MARGIN) {
			localStorage.removeItem(PREFIX + mediaId);
			localStorage.setItem(WATCHED + mediaId, String(Date.now()));
		} else if (t < MIN_START) {
			localStorage.removeItem(PREFIX + mediaId);
		} else {
			const e: Entry = { t: Math.floor(t), at: Date.now() };
			localStorage.setItem(PREFIX + mediaId, JSON.stringify(e));
		}
	} catch {
		/* storage unavailable */
	}
}
