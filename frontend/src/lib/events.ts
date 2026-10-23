import { PUBLIC_API_URL } from '$app/env/public';

// What the backend announces on /api/events (see backend/internal/events and
// migrations/0004): which row changed, or new progress values.
export type ServerEvent =
	| { type: 'media_files'; op: 'INSERT' | 'UPDATE' | 'DELETE'; id: string }
	| { type: 'media_progress'; id: string; progress: number | null }
	| { type: 'torrents'; op: 'INSERT' | 'UPDATE' | 'DELETE'; id: string }
	| {
			type: 'torrent_progress';
			id: string;
			progress: number;
			bytesDone: number;
			bytesTotal: number;
	  }
	| { type: 'rooms'; op: 'INSERT' | 'UPDATE' | 'DELETE'; id: string }
	| { type: 'folders' };

export type Topic = ServerEvent['type'];

/**
 * Gets the events of a burst together. `resync` is true after the socket
 * reconnected: events may have been missed, so reload everything.
 */
type Handler = (events: ServerEvent[], resync: boolean) => void;

interface Listener {
	topics: Topic[];
	fn: Handler;
	pending: ServerEvent[];
	resync: boolean;
	timer?: ReturnType<typeof setTimeout>;
}

const listeners = new Set<Listener>();
let socket: WebSocket | null = null;
let retries = 0;
let retryTimer: ReturnType<typeof setTimeout> | undefined;

// Bursts (e.g. several rows updated by one action) reach the handler as one batch.
function schedule(l: Listener, e: ServerEvent | null) {
	if (e) l.pending.push(e);
	else l.resync = true;
	clearTimeout(l.timer);
	l.timer = setTimeout(() => {
		const events = l.pending;
		const resync = l.resync;
		l.pending = [];
		l.resync = false;
		l.fn(events, resync);
	}, 150);
}

function connect() {
	retryTimer = undefined;
	const ws = new WebSocket(`${PUBLIC_API_URL.replace(/^http/, 'ws')}/api/events`);
	socket = ws;

	ws.onopen = () => {
		// Changes may have been missed while disconnected: resync everyone.
		if (retries > 0) listeners.forEach((l) => schedule(l, null));
		retries = 0;
	};

	ws.onmessage = (msg) => {
		const e = JSON.parse(msg.data) as ServerEvent;
		for (const l of listeners) if (l.topics.includes(e.type)) schedule(l, e);
	};

	ws.onclose = () => {
		if (socket !== ws) return;
		socket = null;
		if (listeners.size === 0) return;
		// count as a retry even after a clean close, so the reopen resyncs
		retryTimer = setTimeout(connect, Math.min(1000 * 2 ** retries++, 10_000));
	};
}

/**
 * Calls `fn` with the batched events of `topics`. Returns an unsubscribe
 * function, so it can be returned from onMount/$effect.
 */
export function onEvents(topics: Topic[], fn: Handler): () => void {
	const l: Listener = { topics, fn, pending: [], resync: false };
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

/** Calls `fn` (debounced) whenever one of `topics` changes, ignoring the details. */
export function onChange(topics: Topic[], fn: () => void): () => void {
	return onEvents(topics, () => fn());
}
