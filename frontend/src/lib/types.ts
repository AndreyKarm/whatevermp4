export type MediaStatus = 'uploading' | 'downloading' | 'processing' | 'ready' | 'failed';
export type TorrentStatus =
	'queued' | 'downloading' | 'importing' | 'done' | 'failed' | 'cancelled';

export interface AudioTrack {
	n: number;
	lang: string;
	title: string;
	codec: string;
	channels: number;
	name: string;
}

export interface SubtitleTrack {
	n: number;
	lang: string;
	title: string;
	isAss: boolean;
	vtt: string;
	ass?: string;
}

/** Timeline preview sprites: `thumbs/sprite_NNN.jpg` (1-based) of cols×rows tiles. */
export interface Thumbnails {
	/** seconds between tiles */
	interval: number;
	width: number;
	height: number;
	cols: number;
	rows: number;
	count: number;
}

export interface MediaItem {
	id: string;
	uploadedBy: string;
	torrentId?: string;
	/** library folder; absent = top level */
	folderId?: string;
	filename: string;
	status: MediaStatus;
	hlsKey?: string;
	posterKey?: string;
	playbackUrl?: string;
	durationSec?: number;
	season?: number;
	episode?: number;
	audioTracks: AudioTrack[];
	subtitleTracks: SubtitleTrack[];
	fonts: string[];
	thumbnails?: Thumbnails;
	/** 0..1 while downloading or processing; absent when idle or unknown */
	progress?: number;
	/** stored original, absent until measured */
	rawBytes?: number;
	/** transcoded output, absent until measured */
	hlsBytes?: number;
	/** the original was deleted: no download or re-process */
	rawDeleted?: boolean;
	error?: string;
	createdAt: string;
}

/** Library folder (a series, a season, …); parentId absent = top level. */
export interface Folder {
	id: string;
	name: string;
	parentId?: string;
	createdAt: string;
}

export interface TorrentItem {
	id: string;
	addedBy: string;
	name: string;
	status: TorrentStatus;
	progress: number;
	bytesDone: number;
	bytesTotal: number;
	error?: string;
	createdAt: string;
}

export interface RoomState {
	mediaId: string;
	playing: boolean;
	position: number;
	updatedAt: number;
	rate: number;
	/** media IDs that play after the current one */
	queue?: string[];
}

export interface RoomMember {
	name: string;
	buffering: boolean;
	/** seconds this viewer is ahead (+) or behind (−) the room clock, from its last status */
	drift?: number;
	/** seconds buffered ahead of its playhead */
	ahead?: number;
}

// Server → client WebSocket messages (backend/internal/rooms/messages.go `Out`).
// The backend omits zero values, so most fields are optional.
export type RoomOut =
	| {
			type: 'state';
			state: RoomState;
			holding?: boolean;
			server: number;
			owner?: string;
			everyone?: boolean;
			controllers?: string[];
	  }
	| { type: 'members'; members?: RoomMember[] }
	| { type: 'chat'; from: RoomMember; text: string }
	| { type: 'pong'; t: number; server: number }
	| { type: 'error'; text: string };

export type ChatMessage = Extract<RoomOut, { type: 'chat' }>;

// Client → server WebSocket messages (backend/internal/rooms/messages.go `In`).
export type RoomIn =
	| { type: 'play' | 'seek'; position: number }
	| { type: 'pause' }
	| { type: 'rate'; rate: number }
	| { type: 'change_media'; mediaId: string; position?: number }
	| { type: 'queue'; queue: string[] }
	| { type: 'next' }
	| { type: 'buffering'; buffering: boolean }
	| { type: 'status'; drift: number; ahead: number }
	| { type: 'chat'; text: string }
	| { type: 'ping'; t: number };

export interface RoomItem {
	id: string;
	name: string;
	ownerName: string;
	everyoneControls: boolean;
	controllers: string[];
	private: boolean;
	/** a password lets people without the invite link in */
	hasPassword: boolean;
	state: RoomState;
	updatedAt: string;
	createdAt: string;
	/** GET /api/rooms/{id}, for the owner of a private room */
	inviteCode?: string;
	/** GET /api/rooms only: nicknames connected right now */
	members?: string[];
	/** GET /api/rooms only: the selected media */
	media?: { id: string; filename: string; posterUrl?: string };
}

/** GET /api/rooms/{id} of a private room without a valid invite. */
export interface LockedRoom {
	id: string;
	name: string;
	private: true;
	locked: true;
	hasPassword: boolean;
}
