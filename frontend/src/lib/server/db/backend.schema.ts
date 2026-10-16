import {
	pgTable,
	text,
	boolean,
	integer,
	bigint,
	doublePrecision,
	timestamp,
	jsonb,
	bigserial,
	customType,
	index
} from 'drizzle-orm/pg-core';
import { user } from './auth.schema';

// Custom bytea type for storing raw .torrent metainfo files
const bytea = customType<{ data: Buffer; notNull: false; default: false }>({
	dataType() {
		return 'bytea';
	}
});

// Shared JSONB types matching Go structs in internal/transcode and internal/rooms
export interface AudioTrack {
	n: number;
	lang: string;
	title: string;
	codec: string;
	channels: number;
	name: string;
}

export interface SubTrack {
	n: number;
	lang: string;
	title: string;
	isAss: boolean;
	vtt: string;
	ass?: string;
}

export interface RoomState {
	mediaId: string;
	playing: boolean;
	position: number;
	updatedAt: number;
	rate: number;
}

// ============================================================================
// Application Tables (Go API + Worker)
// ============================================================================

export const torrents = pgTable('torrents', {
	id: text('id').primaryKey(),
	userId: text('user_id')
		.notNull()
		.references(() => user.id, { onDelete: 'cascade' }),
	name: text('name').notNull().default('Fetching metadata...'),
	magnet: text('magnet'),
	metainfo: bytea('metainfo'),
	status: text('status').notNull().default('queued'), // queued | downloading | importing | done | failed | cancelled
	progress: doublePrecision('progress').notNull().default(0),
	bytesDone: bigint('bytes_done', { mode: 'number' }).notNull().default(0),
	bytesTotal: bigint('bytes_total', { mode: 'number' }).notNull().default(0),
	error: text('error'),
	createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow()
});

export const mediaFiles = pgTable('media_files', {
	id: text('id').primaryKey(),
	uploaderId: text('uploader_id')
		.notNull()
		.references(() => user.id, { onDelete: 'cascade' }),
	torrentId: text('torrent_id').references(() => torrents.id, {
		onDelete: 'set null'
	}),
	filename: text('filename').notNull(),
	status: text('status').notNull().default('uploading'), // uploading | processing | ready | failed
	rawKey: text('raw_key').notNull(),
	hlsKey: text('hls_key'),
	posterKey: text('poster_key'),
	durationSec: doublePrecision('duration_sec'),
	season: integer('season'),
	episode: integer('episode'),
	audioTracks: jsonb('audio_tracks').$type<AudioTrack[]>().notNull().default([]),
	subtitleTracks: jsonb('subtitle_tracks').$type<SubTrack[]>().notNull().default([]),
	fonts: jsonb('fonts').$type<string[]>().notNull().default([]),
	error: text('error'),
	createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow()
});

export const rooms = pgTable('rooms', {
	id: text('id').primaryKey(),
	name: text('name').notNull().default('Watch Party'),
	ownerId: text('owner_id')
		.notNull()
		.references(() => user.id, { onDelete: 'cascade' }),
	everyoneControls: boolean('everyone_controls').notNull().default(false),
	state: jsonb('state').$type<RoomState>().notNull().default({
		mediaId: '',
		playing: false,
		position: 0,
		updatedAt: 0,
		rate: 1
	}),
	updatedAt: timestamp('updated_at', { withTimezone: true }).notNull().defaultNow(),
	createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow()
});

export const jobs = pgTable(
	'jobs',
	{
		id: bigserial('id', { mode: 'number' }).primaryKey(),
		type: text('type').notNull(), // process | torrent
		payload: jsonb('payload').notNull(),
		status: text('status').notNull().default('queued'), // queued | running | done | failed
		attempts: integer('attempts').notNull().default(0),
		lastError: text('last_error'),
		runAt: timestamp('run_at', { withTimezone: true }).notNull().defaultNow(),
		lockedAt: timestamp('locked_at', { withTimezone: true }),
		createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow()
	},
	(t) => [index('jobs_claim_idx').on(t.status, t.runAt)]
);
