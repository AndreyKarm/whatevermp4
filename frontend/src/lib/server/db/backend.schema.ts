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
// BetterAuth + JWKS Tables
// ============================================================================

export const user = pgTable('user', {
	id: text('id').primaryKey(),
	name: text('name').notNull(),
	email: text('email').notNull().unique(),
	emailVerified: boolean('email_verified').notNull().default(false),
	image: text('image'),
	createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
	updatedAt: timestamp('updated_at', { withTimezone: true }).notNull().defaultNow()
});

export const session = pgTable('session', {
	id: text('id').primaryKey(),
	expiresAt: timestamp('expires_at', { withTimezone: true }).notNull(),
	token: text('token').notNull().unique(),
	createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
	updatedAt: timestamp('updated_at', { withTimezone: true }).notNull().defaultNow(),
	ipAddress: text('ip_address'),
	userAgent: text('user_agent'),
	userId: text('user_id')
		.notNull()
		.references(() => user.id, { onDelete: 'cascade' })
});

export const account = pgTable('account', {
	id: text('id').primaryKey(),
	accountId: text('account_id').notNull(),
	providerId: text('provider_id').notNull(),
	userId: text('user_id')
		.notNull()
		.references(() => user.id, { onDelete: 'cascade' }),
	accessToken: text('access_token'),
	refreshToken: text('refresh_token'),
	idToken: text('id_token'),
	accessTokenExpiresAt: timestamp('access_token_expires_at', {
		withTimezone: true
	}),
	refreshTokenExpiresAt: timestamp('refresh_token_expires_at', {
		withTimezone: true
	}),
	scope: text('scope'),
	password: text('password'),
	createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
	updatedAt: timestamp('updated_at', { withTimezone: true }).notNull().defaultNow()
});

export const verification = pgTable('verification', {
	id: text('id').primaryKey(),
	identifier: text('identifier').notNull(),
	value: text('value').notNull(),
	expiresAt: timestamp('expires_at', { withTimezone: true }).notNull(),
	createdAt: timestamp('created_at', { withTimezone: true }).defaultNow(),
	updatedAt: timestamp('updated_at', { withTimezone: true }).defaultNow()
});

export const jwks = pgTable('jwks', {
	id: text('id').primaryKey(),
	publicKey: text('public_key').notNull(),
	privateKey: text('private_key').notNull(),
	createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow()
});

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
