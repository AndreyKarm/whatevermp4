<script lang="ts">
	import { onMount } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { browser } from '$app/env';
	import { PUBLIC_API_URL } from '$app/env/public';
	import {
		Check,
		CheckSquare,
		ChevronRight,
		Download,
		Film,
		Folder as FolderIcon,
		FolderPlus,
		HardDrive,
		Pencil,
		Play,
		Plus,
		Search,
		Upload,
		X
	} from '@lucide/svelte';
	import { apiFetch, apiPost } from '#lib/api.ts';
	import { onChange, onEvents, type ServerEvent } from '#lib/events.ts';
	import { MediaList } from '#lib/media.svelte.ts';
	import { byEpisode, byFilename } from '#lib/episodes.ts';
	import { byNumericName, folderOptions, folderPath, pathLabel, subtree } from '#lib/folders.ts';
	import { getProgress, isWatched } from '#lib/progress.ts';
	import { formatTime } from '#lib/time.ts';
	import { uploader } from '#lib/upload.svelte.ts';
	import { startRoom } from '#lib/watch.ts';
	import type { Folder, MediaItem, MediaStatus, TorrentItem } from '#lib/types.ts';
	import UploadModal from '#lib/components/UploadModal.svelte';
	import MediaEditModal from '#lib/components/MediaEditModal.svelte';
	import FolderModal from '#lib/components/FolderModal.svelte';
	import Downloads from '#lib/components/Downloads.svelte';

	const mediaList = new MediaList();
	const media = $derived(mediaList.items);
	const loaded = $derived(mediaList.loaded);
	let folders = $state<Folder[]>([]);
	let torrents = $state<TorrentItem[]>([]);
	let error = $state<string | null>(null);
	$effect(() => {
		if (mediaList.error) error = mediaList.error;
	});

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

	// ---------- search and filters ----------

	type Scope = 'folder' | 'subfolders' | 'all';
	type WatchFilter = 'all' | 'unwatched' | 'progress' | 'watched';
	let query = $state('');
	let statusFilter = $state<'all' | MediaStatus>('all');
	let watchFilter = $state<WatchFilter>('all');
	let scope = $state<Scope>('folder');
	const needle = $derived(query.trim().toLowerCase());
	const filtering = $derived(
		!!needle || statusFilter !== 'all' || watchFilter !== 'all' || scope !== 'folder'
	);

	// This viewer's saved positions and watched marks (localStorage), re-read whenever the list reloads.
	const positions = $derived(
		browser ? new Map(media.map((m) => [m.id, getProgress(m.id)])) : new Map<string, null>()
	);
	const watchedIds = $derived(
		browser ? new Set(media.filter((m) => isWatched(m.id)).map((m) => m.id)) : new Set<string>()
	);

	function watchState(m: MediaItem): WatchFilter {
		if (positions.get(m.id)) return 'progress';
		return watchedIds.has(m.id) ? 'watched' : 'unwatched';
	}

	// Folders the scope covers (null = top level); undefined = everywhere.
	const scopeFolders = $derived.by((): Set<string | null> | undefined => {
		if (scope === 'folder') return new Set([currentId]);
		// "with subfolders" at the top level is the whole library
		if (scope === 'all' || !currentId) return undefined;
		return subtree(folders, currentId);
	});

	const subfolders = $derived.by(() => {
		if (!filtering) {
			return folders.filter((f) => (f.parentId ?? null) === currentId).sort(byNumericName);
		}
		// while searching, folders whose name matches (within the scope)
		if (!needle) return [];
		return folders
			.filter(
				(f) =>
					f.name.toLowerCase().includes(needle) &&
					(!scopeFolders || scopeFolders.has(f.parentId ?? null))
			)
			.sort(byNumericName);
	});

	const items = $derived.by(() => {
		const list = media.filter((m) => {
			if (scopeFolders && !scopeFolders.has(m.folderId ?? null)) return false;
			if (needle && !m.filename.toLowerCase().includes(needle)) return false;
			if (statusFilter !== 'all' && m.status !== statusFilter) return false;
			if (watchFilter !== 'all' && watchState(m) !== watchFilter) return false;
			return true;
		});
		if (sort === 'name') return list.sort(byFilename);
		if (sort === 'episode') return list.sort(byEpisode);
		return list; // API order: newest first
	});

	function clearFilters() {
		query = '';
		statusFilter = 'all';
		watchFilter = 'all';
		scope = 'folder';
	}

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

	// ---------- loading ----------

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

	// Download progress is patched in; other torrent changes reload the (short) list.
	function onTorrentEvents(events: ServerEvent[], resync: boolean) {
		let reload = resync;
		for (const e of events) {
			if (e.type !== 'torrent_progress') {
				reload = true;
				continue;
			}
			const t = torrents.find((t) => t.id === e.id);
			if (t)
				Object.assign(t, {
					progress: e.progress,
					bytesDone: e.bytesDone,
					bytesTotal: e.bytesTotal
				});
		}
		if (reload) loadTorrents();
	}

	onMount(() => {
		loadFolders();
		loadTorrents();
		const offs = [
			mediaList.live(),
			onChange(['folders'], loadFolders),
			onEvents(['torrents', 'torrent_progress'], onTorrentEvents)
		];
		return () => offs.forEach((off) => off());
	});

	function folderHref(id: string | null) {
		return id ? resolve(`/library?folder=${id}`) : resolve('/library');
	}

	function errorText(e: unknown) {
		return e instanceof Error ? e.message : String(e);
	}

	async function watch(item: MediaItem) {
		try {
			await startRoom(item);
		} catch (e) {
			error = errorText(e);
		}
	}

	// ---------- selection and bulk actions ----------

	let selecting = $state(false);
	const selected = new SvelteSet<string>();
	let bulkBusy = $state(false);
	let bulkTarget = $state('');
	const moveChoices = $derived(folderOptions(folders));

	function toggleSelecting() {
		selecting = !selecting;
		if (!selecting) selected.clear();
	}

	function toggleSelected(id: string) {
		if (selected.has(id)) selected.delete(id);
		else selected.add(id);
	}

	function selectAllVisible() {
		const all = items.every((m) => selected.has(m.id));
		for (const m of items) {
			if (all) selected.delete(m.id);
			else selected.add(m.id);
		}
	}

	type BulkAction = 'move' | 'delete' | 'reprocess' | 'delete_original';

	async function bulk(action: BulkAction, ids = [...selected], folderId: string | null = null) {
		if (!ids.length) return;
		const n = ids.length === 1 ? 'this movie' : `${ids.length} movies`;
		if (action === 'delete' && !confirm(`Delete ${n} for everyone? This can't be undone.`)) return;
		if (
			action === 'delete_original' &&
			!confirm(
				`Delete the original files of ${n}? They keep playing but can't be downloaded or re-processed.`
			)
		)
			return;
		bulkBusy = true;
		try {
			const { skipped } = await apiPost<{ done: number; skipped: number }>('/api/media/bulk', {
				ids,
				action,
				folderId
			});
			error = skipped
				? `${skipped} of ${ids.length} skipped (not ready, still being prepared, or no original).`
				: null;
			if (action === 'delete') for (const id of ids) selected.delete(id);
		} catch (e) {
			error = errorText(e);
		} finally {
			bulkBusy = false;
		}
	}

	function moveSelected() {
		if (!bulkTarget) return;
		bulk('move', [...selected], bulkTarget === 'top' ? null : bulkTarget);
		bulkTarget = '';
	}

	// ---------- dropping files to upload ----------

	// Files dragged in from the desktop: the drop overlay.
	let fileDrag = $state(false);
	let dragDepth = 0;

	const hasFiles = (e: DragEvent) => !!e.dataTransfer?.types.includes('Files');

	function onWindowDragEnter(e: DragEvent) {
		if (!hasFiles(e)) return;
		dragDepth++;
		fileDrag = true;
	}

	function onWindowDragLeave(e: DragEvent) {
		if (!hasFiles(e)) return;
		dragDepth = Math.max(dragDepth - 1, 0);
		if (dragDepth === 0) fileDrag = false;
	}

	function onWindowDragOver(e: DragEvent) {
		// without this the browser opens the dropped file instead
		if (hasFiles(e)) e.preventDefault();
	}

	function onWindowDrop(e: DragEvent) {
		if (!hasFiles(e)) return;
		e.preventDefault();
		dragDepth = 0;
		fileDrag = false;
		const files = [...(e.dataTransfer?.files ?? [])].filter(
			(f) => f.type.startsWith('video/') || /\.(mkv|mp4|m4v|mov|avi|webm|ts)$/i.test(f.name)
		);
		if (!files.length) {
			error = 'Drop video files to upload them.';
			return;
		}
		uploader.enqueue(files, current?.id ?? null);
	}

	function statusText(item: MediaItem, uploading: boolean) {
		if (uploading) return `uploading ${Math.round((uploader.progress ?? 0) * 100)}%`;
		if (item.progress != null && (item.status === 'processing' || item.status === 'downloading'))
			return `${item.status} ${Math.round(item.progress * 100)}%`;
		return item.status;
	}
</script>

<svelte:head><title>{current ? `${current.name} · ` : ''}Library · whatevermp4</title></svelte:head>

<svelte:window
	ondragenter={onWindowDragEnter}
	ondragleave={onWindowDragLeave}
	ondragover={onWindowDragOver}
	ondrop={onWindowDrop}
/>

<main class="container page" class:with-bar={selecting && selected.size > 0}>
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
			<a href={resolve('/library/storage')} class="btn btn-outline" title="Space used">
				<HardDrive size={16} /> Storage
			</a>
			<button
				class="btn btn-outline"
				class:active={selecting}
				aria-pressed={selecting}
				onclick={toggleSelecting}
			>
				<CheckSquare size={16} /> Select
			</button>
			<button class="btn btn-primary" onclick={() => (uploadOpen = true)}>
				<Plus size={16} /> Add movie
			</button>
		</div>
	</div>

	<div class="filters">
		<label class="search">
			<Search size={16} />
			<input
				class="input"
				type="search"
				placeholder="Search by name…"
				aria-label="Search by name"
				bind:value={query}
			/>
		</label>
		<select class="input" aria-label="Where to search" bind:value={scope}>
			<option value="folder">This folder</option>
			<option value="subfolders">With subfolders</option>
			<option value="all">Whole library</option>
		</select>
		<select class="input" aria-label="Status" bind:value={statusFilter}>
			<option value="all">Any status</option>
			<option value="ready">Ready</option>
			<option value="processing">Processing</option>
			<option value="downloading">Downloading</option>
			<option value="uploading">Uploading</option>
			<option value="failed">Failed</option>
		</select>
		<select class="input" aria-label="Watched" bind:value={watchFilter}>
			<option value="all">Watched or not</option>
			<option value="unwatched">Not watched yet</option>
			<option value="progress">In progress</option>
			<option value="watched">Watched</option>
		</select>
		<select class="input" aria-label="Sort" bind:value={sort}>
			<option value="newest">Newest</option>
			<option value="name">Name</option>
			<option value="episode">Season / episode</option>
		</select>
		{#if filtering}
			<button class="btn-icon" title="Clear filters" onclick={clearFilters}><X size={16} /></button>
		{/if}
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
					<a href={folderHref(f.id)} class="thumb folder-thumb" draggable="false">
						{#if info?.poster}<img src={info.poster} alt="" draggable="false" />{/if}
						<span class="folder-icon"><FolderIcon size={36} /></span>
					</a>
					<h3><a href={folderHref(f.id)} title={f.name} draggable="false">{f.name}</a></h3>
					<div class="foot">
						<div class="meta info">
							{#if filtering && f.parentId}<span class="path">{pathLabel(folders, f.parentId)}</span
								>{/if}
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
				{@const watchedAt = positions.get(item.id)}
				{@const uploading = uploader.busy && uploader.mediaId === item.id}
				{@const working =
					!uploading && (item.status === 'processing' || item.status === 'downloading')}
				{@const isSelected = selected.has(item.id)}
				<li class="card" class:selected={isSelected}>
					<div class="thumb">
						{#if item.status === 'ready' && item.posterKey && item.playbackUrl}
							<img src="{PUBLIC_API_URL}{item.playbackUrl}/poster.jpg" alt="" draggable="false" />
						{:else}
							<Film size={32} />
						{/if}
						{#if item.status === 'ready' && !selecting}
							<!-- the whole poster starts (or resumes) a room; the button shows on hover -->
							<button
								class="play"
								title={watchedAt ? `Resume from ${formatTime(watchedAt)}` : 'Watch together'}
								aria-label="{watchedAt ? 'Resume' : 'Watch'} {item.filename}"
								onclick={() => watch(item)}
							>
								<span class="btn-round"><Play size={20} fill="currentColor" /></span>
							</button>
						{/if}
						{#if item.season != null}
							<span class="badge se">S{item.season}E{item.episode ?? '?'}</span>
						{/if}
						{#if watchedIds.has(item.id) && !watchedAt}
							<span class="badge seen" title="Watched"><Check size={10} /></span>
						{/if}
						{#if selecting}
							<button
								class="pick"
								class:on={isSelected}
								aria-label="Select {item.filename}"
								aria-pressed={isSelected}
								onclick={() => toggleSelected(item.id)}
							>
								{#if isSelected}<Check size={14} />{/if}
							</button>
						{/if}
						{#if uploading}
							<div class="overlay">
								<span>{Math.round((uploader.progress ?? 0) * 100)}%</span>
								<div class="progress">
									<div style:width="{(uploader.progress ?? 0) * 100}%"></div>
								</div>
							</div>
						{:else if working}
							<div class="overlay">
								{#if item.progress != null}
									<span>{Math.round(item.progress * 100)}%</span>
									<div class="progress"><div style:width="{item.progress * 100}%"></div></div>
								{:else}
									<span>{item.status}…</span>
									<div class="progress indeterminate"><div></div></div>
								{/if}
							</div>
						{:else if watchedAt && item.durationSec}
							<div class="watched" title="Stopped at {formatTime(watchedAt)}">
								<div style:width="{(watchedAt / item.durationSec) * 100}%"></div>
							</div>
						{/if}
					</div>
					<h3 title={item.filename}>{item.filename}</h3>
					<div class="foot">
						<div class="meta info">
							{#if filtering && item.folderId && item.folderId !== currentId}
								<a class="path" href={folderHref(item.folderId)}
									>{pathLabel(folders, item.folderId)}</a
								>
							{/if}
							<span class="status-{item.status}">{statusText(item, uploading)}</span>
							<span class="by">by {item.uploadedBy}</span>
						</div>
						{#if item.status !== 'uploading' && item.status !== 'downloading' && !item.rawDeleted}
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
					</div>
				</li>
			{/each}
		</ul>
	{:else if loaded && !error && filtering}
		<div class="panel empty-state">
			<Search size={40} />
			<p>Nothing matches.</p>
			<button class="btn btn-outline" onclick={clearFilters}>Clear filters</button>
		</div>
	{:else if loaded && !error}
		<div class="panel empty-state">
			<Film size={40} />
			<p>{current ? 'This folder is empty.' : 'The library is empty.'}</p>
			<p class="muted">
				Upload a file (or drop it here), add a torrent or import an .m3u8 link{current
					? ' into this folder'
					: ''}, or create folders for a series and its seasons.
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

{#if selecting}
	<div class="bulk-bar" role="toolbar" aria-label="Selected movies">
		<div class="container bulk-inner">
			<span class="meta count">{selected.size} selected</span>
			<button class="btn btn-outline" onclick={selectAllVisible} disabled={!items.length}>
				{items.length && items.every((m) => selected.has(m.id)) ? 'None' : 'All'}
			</button>
			<select
				class="input"
				aria-label="Move to folder"
				bind:value={bulkTarget}
				onchange={moveSelected}
				disabled={!selected.size || bulkBusy}
			>
				<option value="" disabled>Move to…</option>
				<option value="top">— Top level —</option>
				{#each moveChoices as o (o.id)}
					<option value={o.id}>{o.label}</option>
				{/each}
			</select>
			<button
				class="btn btn-outline"
				disabled={!selected.size || bulkBusy}
				onclick={() => bulk('reprocess')}
			>
				Re-process
			</button>
			<button
				class="btn btn-outline"
				disabled={!selected.size || bulkBusy}
				onclick={() => bulk('delete_original')}
			>
				Delete originals
			</button>
			<button
				class="btn btn-danger"
				disabled={!selected.size || bulkBusy}
				onclick={() => bulk('delete')}
			>
				Delete
			</button>
			<button class="btn-icon" title="Done" onclick={toggleSelecting}><X size={18} /></button>
		</div>
	</div>
{/if}

{#if fileDrag}
	<div class="drop-overlay">
		<Upload size={40} />
		<p>Drop to upload into {current ? current.name : 'the library'}</p>
	</div>
{/if}

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
	.with-bar {
		padding-bottom: 6rem;
	}

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

	.crumbs a {
		padding: 0 0.25rem;
		border-radius: 0.25rem;
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
	.filters .input {
		height: 2.375rem;
	}

	.tools .btn {
		padding: 0 1rem;
	}

	.tools .btn.active {
		border-color: var(--color-primary);
		color: var(--color-primary);
	}

	.filters {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.75rem;
		margin: -0.75rem 0 2rem;
	}

	.filters .input {
		width: auto;
		padding: 0 0.75rem;
	}

	.search {
		position: relative;
		display: flex;
		flex: 1 1 14rem;
		align-items: center;
		color: var(--color-gray);
	}

	.search :global(svg) {
		position: absolute;
		left: 0.75rem;
		pointer-events: none;
	}

	.filters .search .input {
		width: 100%;
		padding-left: 2.25rem;
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
		border-radius: 0.3rem;
	}

	.card.selected .thumb {
		box-shadow: var(--ring);
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

	/* the whole poster is the play button; the round icon appears on hover */
	.play {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		background-color: rgb(0 0 0 / 0);
		transition: background-color 0.2s ease;
	}

	.play .btn-round {
		width: 3rem;
		height: 3rem;
		opacity: 0;
		transform: scale(0.85);
		transition:
			opacity 0.2s ease,
			transform 0.2s ease;
	}

	.play:hover,
	.play:focus-visible {
		background-color: rgb(0 0 0 / 0.35);
	}

	.play:hover .btn-round,
	.play:focus-visible .btn-round {
		opacity: 1;
		transform: scale(1);
	}

	/* touch screens can't hover: keep the button visible */
	@media (hover: none) {
		.play .btn-round {
			opacity: 0.85;
			transform: none;
		}
	}

	.se {
		position: absolute;
		top: 0.5rem;
		right: 0.5rem;
	}

	.seen {
		position: absolute;
		bottom: 0.5rem;
		right: 0.5rem;
		display: flex;
		padding: 0.2rem;
	}

	/* selection checkbox in the poster's corner */
	.pick {
		position: absolute;
		top: 0.5rem;
		left: 0.5rem;
		z-index: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 1.5rem;
		height: 1.5rem;
		border: 2px solid var(--color-text);
		border-radius: 0.35rem;
		background-color: rgb(0 0 0 / 0.5);
		color: white;
	}

	.pick.on {
		border-color: var(--color-primary);
		background-color: var(--color-primary);
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

	/* upload / import / transcode progress over the poster */
	.overlay {
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
		text-transform: uppercase;
	}

	.indeterminate > div {
		width: 30%;
		animation: slide 1.2s ease-in-out infinite;
	}

	@keyframes slide {
		from {
			transform: translateX(-100%);
		}
		to {
			transform: translateX(340%);
		}
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

	.by,
	.path {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		text-transform: none;
	}

	.path {
		color: var(--color-muted);
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

	.bulk-bar {
		position: fixed;
		right: 0;
		bottom: 0;
		left: 0;
		z-index: 15;
		padding: 0.75rem 0;
		border-top: 1px solid var(--color-border);
		background-color: color-mix(in srgb, var(--color-secondary), transparent 5%);
		backdrop-filter: blur(10px);
	}

	.bulk-inner {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.75rem;
	}

	.bulk-inner .btn,
	.bulk-inner .input {
		height: 2.25rem;
		padding: 0 0.9rem;
	}

	.bulk-inner .input {
		width: auto;
		max-width: 14rem;
	}

	.count {
		margin-right: auto;
		font-size: 12px;
	}

	.drop-overlay {
		position: fixed;
		inset: 0;
		z-index: 30;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 1rem;
		border: 3px dashed var(--color-primary);
		background-color: color-mix(in srgb, var(--color-bg), transparent 15%);
		color: var(--color-primary);
		font-weight: 700;
		text-transform: uppercase;
		pointer-events: none;
	}
</style>
