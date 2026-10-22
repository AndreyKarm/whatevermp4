// Per-viewer watch positions, kept only in this browser's localStorage.
// Storage can be unavailable (private mode, blocked site data); everything
// here then quietly does nothing.

const PREFIX = 'progress:';

// Positions this close to the start or end don't count as "in progress".
const MIN_START = 10;
const END_MARGIN = 60;

interface Entry {
	t: number;
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

/** Saves the position; near the start or end it clears it instead (not started / finished). */
export function saveProgress(mediaId: string, t: number, duration: number) {
	try {
		if (t < MIN_START || (duration > 0 && duration - t < END_MARGIN)) {
			localStorage.removeItem(PREFIX + mediaId);
		} else {
			const e: Entry = { t: Math.floor(t), at: Date.now() };
			localStorage.setItem(PREFIX + mediaId, JSON.stringify(e));
		}
	} catch {
		/* storage unavailable */
	}
}
