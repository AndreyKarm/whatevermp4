import { PUBLIC_API_URL } from '$app/env/public';

// Table names the backend announces on /api/events (see backend/internal/events).
export type Topic = 'media_files' | 'torrents' | 'rooms';

interface Listener {
	topics: Topic[];
	fn: () => void;
	timer?: ReturnType<typeof setTimeout>;
}

const listeners = new Set<Listener>();
let socket: WebSocket | null = null;
let retries = 0;
let retryTimer: ReturnType<typeof setTimeout> | undefined;

// Bursts (e.g. torrent progress, several rows updated) collapse into one refetch.
function schedule(l: Listener) {
	clearTimeout(l.timer);
	l.timer = setTimeout(l.fn, 250);
}

function connect() {
	const ws = new WebSocket(`${PUBLIC_API_URL.replace(/^http/, 'ws')}/api/events`);
	socket = ws;

	ws.onopen = () => {
		// Changes may have been missed while disconnected: resync everyone.
		if (retries > 0) listeners.forEach(schedule);
		retries = 0;
	};

	ws.onmessage = (e) => {
		const { type } = JSON.parse(e.data) as { type: Topic };
		for (const l of listeners) if (l.topics.includes(type)) schedule(l);
	};

	ws.onclose = () => {
		if (socket !== ws) return;
		socket = null;
		if (listeners.size === 0) return;
		retryTimer = setTimeout(connect, Math.min(1000 * 2 ** retries++, 10_000));
	};
}

/**
 * Calls `fn` (debounced) whenever one of `topics` changes on the server.
 * Returns an unsubscribe function, so it can be returned from onMount/$effect.
 */
export function onChange(topics: Topic[], fn: () => void): () => void {
	const l: Listener = { topics, fn };
	listeners.add(l);
	if (!socket && !retryTimer) connect();

	return () => {
		listeners.delete(l);
		clearTimeout(l.timer);
		if (listeners.size === 0) {
			clearTimeout(retryTimer);
			retryTimer = undefined;
			const ws = socket;
			socket = null;
			ws?.close();
		}
	};
}
