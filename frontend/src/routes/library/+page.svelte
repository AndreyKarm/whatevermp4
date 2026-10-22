<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { browser } from '$app/env';
	import { PUBLIC_API_URL } from '$app/env/public';
	import {
		ChevronRight,
		Download,
		Film,
		Folder as FolderIcon,
		FolderPlus,
		Pencil,
		Play,
		Plus
	} from '@lucide/svelte';
	import { apiFetch, apiPost } from '#lib/api.ts';
	import { onChange } from '#lib/events.ts';
	import { byNumericName, folderPath, subtree } from '#lib/folders.ts';
	import { getProgress } from '#lib/progress.ts';
	import { formatTime } from '#lib/time.ts';
	import { uploader } from '#lib/upload.svelte.ts';
	import type { Folder, MediaItem, TorrentItem } from '#lib/types.ts';
	import UploadModal from '#lib/components/UploadModal.svelte';
	import MediaEditModal from '#lib/components/MediaEditModal.svelte';
	import FolderModal from '#lib/components/FolderModal.svelte';
	import Downloads from '#lib/components/Downloads.svelte';

	let media = $state<MediaItem[]>([]);
	let folders = $state<Folder[]>([]);
	let torrents = $state<TorrentItem[]>([]);
	let loaded = $state(false);
	let error = $state<string | null>(null);

	// Current folder from ?folder=…; null = top level.
	const currentId = $derived(page.url.searchParams.get('folder'));
	const current = $derived(folders.find((f) => f.id === currentId) ?? null);
	const missing = $derived(loaded && !!currentId && !current);
	const crumbs = $derived(folderPath(folders, currentId));

	let uploadOpen = $state(false);
	let editingId = $state<string | null>(null);
	// Look the item up live so the modal reflects status changes pushed by the server.
	const editing = $derived(media.find((m) => m.id === editingId) ?? null);

	// Folder modal: { folder: null } creates one in the current folder.
	let folderModal = $state<{ folder: Folder | null } | null>(null);

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

	const subfolders = $derived(
		folders.filter((f) => (f.parentId ?? null) === currentId).sort(byNumericName)
	);

	const items = $derived.by(() => {
		const list = media.filter((m) => (m.folderId ?? null) === currentId);
		if (sort === 'name') return list.sort(byName);
		if (sort === 'episode') {
			// Items with season/episode first, in order; the rest by name.
			const key = (m: MediaItem) => (m.season ?? Infinity) * 10_000 + (m.episode ?? 0);
			return list.sort((a, b) => key(a) - key(b) || byName(a, b));
		}
		return list; // API order: newest first
	});

	// Per folder card: videos anywhere inside it, direct subfolders, and a poster to show.
	const folderInfo = $derived(
		new Map(
			subfolders.map((f) => {
				const ids = subtree(folders, f.id);
				const inside = media.filter((m) => m.folderId && ids.has(m.folderId));
				const cover = inside.find((m) => m.status === 'ready' && m.posterKey && m.playbackUrl);
				return [
					f.id,
					{
						videos: inside.length,
						folders: folders.filter((c) => c.parentId === f.id).length,
						poster: cover ? `${PUBLIC_API_URL}${cover.playbackUrl}/poster.jpg` : null
					}
				];
			})
		)
	);

	// This viewer's saved positions (localStorage), re-read whenever the list reloads.
	const progress = $derived(
		browser ? new Map(media.map((m) => [m.id, getProgress(m.id)])) : new Map<string, null>()
	);

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

	async function loadFolders() {
		try {
			folders = await apiFetch<Folder[]>('/api/folders');
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
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
		loadFolders();
		loadTorrents();
		const offs = [
			onChange(['media_files'], loadMedia),
			onChange(['folders'], loadFolders),
			onChange(['torrents'], loadTorrents)
		];
		return () => offs.forEach((off) => off());
	});

	function folderHref(id: string | null) {
		return id ? resolve(`/library?folder=${id}`) : resolve('/library');
	}

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

<svelte:head><title>{current ? `${current.name} · ` : ''}Library · whatevermp4</title></svelte:head>

<main class="container page">
	<div class="page-head">
		<div class="title">
			<h1 class="section-title">{current?.name ?? 'Library'}</h1>
			{#if crumbs.length}
				<nav class="crumbs" aria-label="Folder path">
					<a href={folderHref(null)}>Library</a>
					{#each crumbs.slice(0, -1) as c (c.id)}
						<ChevronRight size={14} />
						<a href={folderHref(c.id)}>{c.name}</a>
					{/each}
					<ChevronRight size={14} />
					<span>{current?.name}</span>
				</nav>
			{:else}
				<p class="muted">Every movie available to watch. Press play to start a room.</p>
			{/if}
		</div>
		<div class="tools">
			{#if current}
				<button
					class="btn btn-outline"
					title="Rename, move or delete this folder"
					onclick={() => (folderModal = { folder: current })}
				>
					<Pencil size={14} /> Folder
				</button>
			{/if}
			<button class="btn btn-outline" onclick={() => (folderModal = { folder: null })}>
				<FolderPlus size={16} /> New folder
			</button>
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

	{#if missing}
		<div class="panel empty-state">
			<FolderIcon size={40} />
			<p>This folder no longer exists.</p>
			<a href={folderHref(null)} class="btn btn-primary">Back to the library</a>
		</div>
	{:else if subfolders.length || items.length}
		<ul class="grid">
			{#each subfolders as f (f.id)}
				{@const info = folderInfo.get(f.id)}
				<li class="card">
					<a href={folderHref(f.id)} class="thumb folder-thumb">
						{#if info?.poster}<img src={info.poster} alt="" draggable="false" />{/if}
						<span class="folder-icon"><FolderIcon size={36} /></span>
					</a>
					<h3><a href={folderHref(f.id)} title={f.name}>{f.name}</a></h3>
					<div class="foot">
						<div class="meta info">
							{#if info?.folders}
								<span>{info.folders} folder{info.folders === 1 ? '' : 's'}</span>
							{/if}
							<span>{info?.videos ?? 0} video{info?.videos === 1 ? '' : 's'}</span>
						</div>
						<button
							class="btn-icon edit"
							title="Edit folder"
							onclick={() => (folderModal = { folder: f })}
						>
							<Pencil size={16} />
						</button>
					</div>
				</li>
			{/each}

			{#each items as item (item.id)}
				{@const watched = progress.get(item.id)}
				{@const uploading = uploader.busy && uploader.mediaId === item.id}
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
						{#if uploading}
							<div class="uploading">
								<span>{Math.round((uploader.progress ?? 0) * 100)}%</span>
								<div class="progress">
									<div style:width="{(uploader.progress ?? 0) * 100}%"></div>
								</div>
							</div>
						{:else if watched && item.durationSec}
							<div class="watched" title="Stopped at {formatTime(watched)}">
								<div style:width="{(watched / item.durationSec) * 100}%"></div>
							</div>
						{/if}
					</div>
					<h3 title={item.filename}>{item.filename}</h3>
					<div class="foot">
						<div class="meta info">
							<span class="status-{item.status}">
								{item.status}{#if uploading}&nbsp;{Math.round((uploader.progress ?? 0) * 100)}%{/if}
							</span>
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
			<p>{current ? 'This folder is empty.' : 'The library is empty.'}</p>
			<p class="muted">
				Upload a file, add a torrent or import an .m3u8 link{current ? ' into this folder' : ''}, or
				create folders for a series and its seasons.
			</p>
			<div class="empty-actions">
				<button class="btn btn-outline" onclick={() => (folderModal = { folder: null })}>
					<FolderPlus size={16} /> New folder
				</button>
				<button class="btn btn-primary" onclick={() => (uploadOpen = true)}>
					<Plus size={16} /> Add movie
				</button>
			</div>
		</div>
	{/if}
</main>

<UploadModal
	open={uploadOpen}
	onclose={() => (uploadOpen = false)}
	folderId={current?.id ?? null}
	folderLabel={crumbs.map((c) => c.name).join(' / ')}
/>
<MediaEditModal
	item={editing}
	{folders}
	onclose={() => (editingId = null)}
	onwatch={(item) => {
		editingId = null;
		watch(item);
	}}
/>
<FolderModal
	open={!!folderModal}
	folder={folderModal?.folder ?? null}
	parentId={current?.id ?? null}
	{folders}
	onclose={() => (folderModal = null)}
	ondeleted={(parentId) => {
		// leave a folder that was just deleted
		if (!folders.some((f) => f.id === currentId)) goto(folderHref(parentId));
	}}
/>

<style>
	.title {
		min-width: 0;
	}

	.crumbs {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.25rem;
		font-size: 14px;
		color: var(--color-muted);
	}

	.crumbs span {
		color: var(--color-text);
	}

	.tools {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.75rem;
	}

	/* every control in the toolbar shares one height */
	.tools .btn,
	.tools .input {
		height: 2.375rem;
	}

	.tools .btn {
		padding: 0 1rem;
	}

	.sort {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.sort .input {
		width: auto;
		padding: 0 0.75rem;
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
	}

	.thumb img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	/* folders: dimmed poster from inside with a folder icon on top */
	.folder-thumb img {
		position: absolute;
		inset: 0;
		opacity: 0.35;
	}

	.folder-icon {
		position: relative;
		color: var(--color-text);
	}

	.folder-thumb:hover .folder-icon {
		color: var(--color-primary);
	}

	h3 a:hover {
		color: var(--color-primary);
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

	.uploading {
		position: absolute;
		inset: 0;
		display: flex;
		flex-direction: column;
		justify-content: center;
		gap: 0.5rem;
		padding: 0 1rem;
		background-color: rgb(0 0 0 / 0.55);
		color: var(--color-text);
		font-size: 13px;
		font-weight: 700;
		text-align: center;
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

	.empty-actions {
		display: flex;
		gap: 0.75rem;
	}
</style>
