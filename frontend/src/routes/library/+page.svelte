<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { browser } from '$app/env';
	import { PUBLIC_API_URL } from '$app/env/public';
	import { Download, Film, Pencil, Play, Plus } from '@lucide/svelte';
	import { apiFetch, apiPost } from '#lib/api.ts';
	import { onChange } from '#lib/events.ts';
	import { getProgress } from '#lib/progress.ts';
	import { formatTime } from '#lib/time.ts';
	import type { MediaItem, TorrentItem } from '#lib/types.ts';
	import UploadModal from '#lib/components/UploadModal.svelte';
	import MediaEditModal from '#lib/components/MediaEditModal.svelte';
	import Downloads from '#lib/components/Downloads.svelte';

	let media = $state<MediaItem[]>([]);
	let torrents = $state<TorrentItem[]>([]);
	let loaded = $state(false);
	let error = $state<string | null>(null);

	let uploadOpen = $state(false);
	let editingId = $state<string | null>(null);
	// Look the item up live so the modal reflects status changes pushed by the server.
	const editing = $derived(media.find((m) => m.id === editingId) ?? null);

	type Sort = 'newest' | 'name' | 'episode';
	const SORT_KEY = 'library-sort';
	let sort = $state<Sort>(readSort());

	function readSort(): Sort {
		if (!browser) return 'newest';
		try {
			const v = localStorage.getItem(SORT_KEY);
			return v === 'name' || v === 'episode' ? v : 'newest';
		} catch {
			return 'newest';
		}
	}

	$effect(() => {
		try {
			localStorage.setItem(SORT_KEY, sort);
		} catch {
			/* storage unavailable: the choice just isn't remembered */
		}
	});

	const byName = (a: MediaItem, b: MediaItem) =>
		a.filename.localeCompare(b.filename, undefined, { numeric: true, sensitivity: 'base' });

	const sorted = $derived.by(() => {
		const list = [...media];
		if (sort === 'name') return list.sort(byName);
		if (sort === 'episode') {
			// Items with season/episode first, in order; the rest by name.
			const key = (m: MediaItem) => (m.season ?? Infinity) * 10_000 + (m.episode ?? 0);
			return list.sort((a, b) => key(a) - key(b) || byName(a, b));
		}
		return list; // API order: newest first
	});

	async function loadMedia() {
		try {
			media = await apiFetch<MediaItem[]>('/api/media');
			error = null;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loaded = true;
		}
	}

	async function loadTorrents() {
		try {
			torrents = await apiFetch<TorrentItem[]>('/api/torrents');
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}

	onMount(() => {
		loadMedia();
		loadTorrents();
		const offMedia = onChange(['media_files'], loadMedia);
		const offTorrents = onChange(['torrents'], loadTorrents);
		return () => {
			offMedia();
			offTorrents();
		};
	});

	// This viewer's saved positions (localStorage), re-read whenever the list reloads.
	const progress = $derived(
		browser ? new Map(media.map((m) => [m.id, getProgress(m.id)])) : new Map<string, null>()
	);

	async function watch(item: MediaItem) {
		try {
			const { id } = await apiPost<{ id: string }>('/api/rooms', {
				name: item.filename,
				mediaId: item.id,
				// the room starts where this host last stopped
				position: getProgress(item.id) ?? 0
			});
			goto(resolve('/rooms/[id]', { id }));
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}
</script>

<svelte:head><title>Library · whatevermp4</title></svelte:head>

<main class="container page">
	<div class="page-head">
		<div>
			<h1 class="section-title">Library</h1>
			<p class="muted">Every movie available to watch. Press play to start a room.</p>
		</div>
		<div class="tools">
			<label class="sort">
				<span class="label">Sort</span>
				<select class="input" bind:value={sort}>
					<option value="newest">Newest</option>
					<option value="name">Name</option>
					<option value="episode">Season / episode</option>
				</select>
			</label>
			<button class="btn btn-primary" onclick={() => (uploadOpen = true)}>
				<Plus size={16} /> Add movie
			</button>
		</div>
	</div>

	{#if error}<p class="error-text">{error}</p>{/if}

	<Downloads {torrents} />

	{#if sorted.length}
		<ul class="grid">
			{#each sorted as item (item.id)}
				{@const watched = progress.get(item.id)}
				<li class="card">
					<div class="thumb">
						{#if item.status === 'ready' && item.posterKey && item.playbackUrl}
							<img src="{PUBLIC_API_URL}{item.playbackUrl}/poster.jpg" alt="" draggable="false" />
						{:else}
							<Film size={32} />
						{/if}
						{#if item.season != null}
							<span class="badge se">S{item.season}E{item.episode ?? '?'}</span>
						{/if}
						{#if watched && item.durationSec}
							<div class="watched" title="Stopped at {formatTime(watched)}">
								<div style:width="{(watched / item.durationSec) * 100}%"></div>
							</div>
						{/if}
					</div>
					<h3 title={item.filename}>{item.filename}</h3>
					<div class="foot">
						<div class="meta info">
							<span class="status-{item.status}">{item.status}</span>
							<span class="by">by {item.uploadedBy}</span>
						</div>
						{#if item.status !== 'uploading' && item.status !== 'downloading'}
							<!-- eslint-disable svelte/no-navigation-without-resolve -- API host, not an app route -->
							<a
								class="btn-icon edit"
								title="Download original"
								href="{PUBLIC_API_URL}/api/media/{item.id}/download"
								download
							>
								<Download size={16} />
							</a>
							<!-- eslint-enable svelte/no-navigation-without-resolve -->
						{/if}
						<button class="btn-icon edit" title="Edit" onclick={() => (editingId = item.id)}>
							<Pencil size={16} />
						</button>
						{#if item.status === 'ready'}
							<button
								class="btn-round"
								title={watched ? `Resume from ${formatTime(watched)}` : 'Watch together'}
								onclick={() => watch(item)}
							>
								<Play size={14} fill="currentColor" />
							</button>
						{/if}
					</div>
				</li>
			{/each}
		</ul>
	{:else if loaded && !error}
		<div class="panel empty-state">
			<Film size={40} />
			<p>The library is empty.</p>
			<p class="muted">Upload a file, add a torrent or import an .m3u8 link.</p>
			<button class="btn btn-primary" onclick={() => (uploadOpen = true)}>
				<Plus size={16} /> Add movie
			</button>
		</div>
	{/if}
</main>

<UploadModal open={uploadOpen} onclose={() => (uploadOpen = false)} />
<MediaEditModal
	item={editing}
	onclose={() => (editingId = null)}
	onwatch={(item) => {
		editingId = null;
		watch(item);
	}}
/>

<style>
	.tools {
		display: flex;
		align-items: flex-end;
		gap: 0.75rem;
	}

	.sort {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.sort .input {
		width: auto;
		padding: 0.5rem 0.75rem;
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(13.5rem, 1fr));
		gap: 1.75rem 1.5rem;
	}

	.card {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		min-width: 0;
	}

	.thumb {
		position: relative;
		isolation: isolate;
		display: flex;
		align-items: center;
		justify-content: center;
		aspect-ratio: 16 / 9;
		overflow: hidden;
		border-radius: 0.2rem;
		background-color: var(--color-secondary);
		color: var(--color-muted);
		transition: box-shadow 0.3s ease;
	}

	.thumb img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.card:hover .thumb {
		box-shadow: var(--glow);
	}

	.se {
		position: absolute;
		top: 0.5rem;
		right: 0.5rem;
	}

	/* how far this viewer got, along the bottom of the poster */
	.watched {
		position: absolute;
		right: 0;
		bottom: 0;
		left: 0;
		height: 4px;
		background-color: rgb(0 0 0 / 0.5);
	}

	.watched > div {
		height: 100%;
		background-color: var(--color-primary);
	}

	h3 {
		display: -webkit-box;
		overflow: hidden;
		font-size: 12px;
		font-weight: 700;
		line-height: 1.4;
		text-transform: uppercase;
		overflow-wrap: anywhere;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
	}

	.foot {
		display: flex;
		align-items: center;
		gap: 0.25rem;
		margin-top: auto;
	}

	.info {
		display: flex;
		flex: 1;
		flex-direction: column;
		min-width: 0;
	}

	.by {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		text-transform: none;
	}

	.edit {
		color: var(--color-gray);
	}

	.empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.75rem;
		padding: 3rem 1.5rem;
		text-align: center;
		color: var(--color-muted);
	}

	.empty-state p:first-of-type {
		color: var(--color-text);
		font-weight: 700;
		text-transform: uppercase;
	}
</style>
