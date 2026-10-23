import type { ChatMessage, RoomIn, RoomMember, RoomOut, RoomState } from './types';

// The backend closes with 1008 (policy violation) when the room doesn't exist or was deleted.
const CLOSE_ROOM_GONE = 1008;
// ...and with 4003 when a private room is joined without a valid invite code.
const CLOSE_INVITE_REQUIRED = 4003;

const INVITE_PREFIX = 'room-invite:';

/** The invite code remembered for a private room in this browser. */
export function savedInvite(roomId: string): string | null {
	try {
		return localStorage.getItem(INVITE_PREFIX + roomId);
	} catch {
		return null;
	}
}

export function saveInvite(roomId: string, code: string) {
	try {
		localStorage.setItem(INVITE_PREFIX + roomId, code);
	} catch {
		/* storage unavailable: the link has to be opened again */
	}
}

export class RoomSocket {
	state = $state<RoomState | null>(null);
	owner = $state('');
	everyone = $state(false);
	controllers = $state<string[]>([]);
	members = $state<RoomMember[]>([]);
	chat = $state<ChatMessage[]>([]);
	isHolding = $state(false);
	connected = $state(false);
	/** Set when the room is gone; no reconnect is attempted. */
	fatal = $state<string | null>(null);
	/** Set when a private room refused us: a password or invite link is needed. */
	locked = $state(false);
	/** Last transient error from the server (e.g. not allowed to control). */
	error = $state<string | null>(null);

	#ws: WebSocket | null = null;
	#offset = 0;
	#rtt = 0;
	#nickname: string;
	#roomId: string;
	#apiUrl: string;
	#invite: string | null;
	#pingTimer: ReturnType<typeof setInterval> | null = null;
	#retryTimer: ReturnType<typeof setTimeout> | null = null;
	#retries = 0;
	#closed = false;

	constructor(roomId: string, nickname: string, apiUrl: string, invite: string | null = null) {
		this.#roomId = roomId;
		this.#nickname = nickname;
		this.#apiUrl = apiUrl;
		this.#invite = invite;
	}

	get canControl() {
		return (
			this.everyone || this.owner === this.#nickname || this.controllers.includes(this.#nickname)
		);
	}

	get queue(): string[] {
		return this.state?.queue ?? [];
	}

	serverNow() {
		return Date.now() + this.#offset;
	}

	getTargetPosition(): number {
		if (!this.state) return 0;
		const elapsed = (this.serverNow() - this.state.updatedAt) / 1000;
		return this.state.position + (this.state.playing ? elapsed * this.state.rate : 0);
	}

	connect() {
		this.#closed = false;
		const invite = this.#invite ? `&invite=${encodeURIComponent(this.#invite)}` : '';
		const url = `${this.#apiUrl.replace(/^http/, 'ws')}/api/rooms/${this.#roomId}/ws?nickname=${encodeURIComponent(this.#nickname)}${invite}`;
		const ws = new WebSocket(url);
		this.#ws = ws;

		ws.onopen = () => {
			this.connected = true;
			this.#retries = 0;
			this.#ping();
		};

		ws.onmessage = (e) => this.#handle(JSON.parse(e.data) as RoomOut);

		ws.onclose = (e) => {
			if (this.#ws !== ws) return;
			this.connected = false;
			this.#ws = null;
			if (this.#closed) return;
			if (e.code === CLOSE_ROOM_GONE) {
				this.fatal = e.reason || 'room not found';
				return;
			}
			if (e.code === CLOSE_INVITE_REQUIRED) {
				this.locked = true;
				return;
			}
			const delay = Math.min(1000 * 2 ** this.#retries++, 10_000);
			this.#retryTimer = setTimeout(() => this.connect(), delay);
		};

		this.#pingTimer ??= setInterval(() => this.#ping(), 5000);
	}

	close() {
		this.#closed = true;
		if (this.#pingTimer) clearInterval(this.#pingTimer);
		if (this.#retryTimer) clearTimeout(this.#retryTimer);
		this.#pingTimer = this.#retryTimer = null;
		this.#ws?.close();
		this.#ws = null;
		this.connected = false;
	}

	send(payload: RoomIn) {
		if (this.#ws?.readyState === WebSocket.OPEN) this.#ws.send(JSON.stringify(payload));
	}

	// Each ping carries the previous one's round trip, which others see as our ping.
	#ping() {
		this.send({ type: 'ping', t: Date.now(), rtt: this.#rtt || undefined });
	}

	#handle(msg: RoomOut) {
		switch (msg.type) {
			case 'state':
				this.state = msg.state;
				this.isHolding = msg.holding ?? false;
				this.owner = msg.owner ?? '';
				this.everyone = msg.everyone ?? false;
				this.controllers = msg.controllers ?? [];
				break;
			case 'members':
				this.members = msg.members ?? [];
				break;
			case 'chat':
				this.chat.push(msg);
				break;
			case 'pong': {
				const rtt = Date.now() - msg.t;
				this.#rtt = Math.max(rtt, 1);
				this.#offset = msg.server - (msg.t + rtt / 2);
				break;
			}
			case 'error':
				this.error = msg.text;
				setTimeout(() => {
					if (this.error === msg.text) this.error = null;
				}, 4000);
				break;
		}
	}
}
