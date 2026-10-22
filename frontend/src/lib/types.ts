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

export interface MediaItem {
	id: string;
	uploadedBy: string;
	torrentId?: string;
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
	error?: string;
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
}

export interface RoomMember {
	name: string;
	buffering: boolean;
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
	| { type: 'change_media'; mediaId: string }
	| { type: 'buffering'; buffering: boolean }
	| { type: 'chat'; text: string }
	| { type: 'ping'; t: number };

export interface RoomItem {
	id: string;
	name: string;
	ownerName: string;
	everyoneControls: boolean;
	state: RoomState;
	updatedAt: string;
	createdAt: string;
}
