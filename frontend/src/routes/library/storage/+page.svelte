<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { ArrowLeft, FileX, Folder as FolderIcon, HardDrive } from '@lucide/svelte';
	import { apiFetch, apiPost } from '#lib/api.ts';
	import { formatBytes } from '#lib/bytes.ts';
	import { onChange } from '#lib/events.ts';
	import { byNumericName, subtree } from '#lib/folders.ts';
	import type { Folder, MediaItem } from '#lib/types.ts';

	let media = $state<MediaItem[]>([]);
	let folders = $state<Folder[]>([]);
	let error = $state<string | null>(null);
	let busy = $state(false);

	async function load() {
		try {
			[media, folders] = await Promise.all([
				apiFetch<MediaItem[]>('/api/media'),
				apiFetch<Folder[]>('/api/folders')
			]);
			error = null;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}

	onMount(() => {
		load();
		return onChange(['media_files', 'folders'], load);
	});

	const size = (m: MediaItem) => (m.rawBytes ?? 0) + (m.hlsBytes ?? 0);
	// originals that can still be deleted: ready, stored, not gone yet
	const deletable = (m: MediaItem) => m.status === 'ready' && !m.rawDeleted;

	interface Totals {
		raw: number;
		hls: number;
		count: number;
		originals: string[];
	}

	function totals(list: MediaItem[]): Totals {
		return {
			raw: list.reduce((n, m) => n + (m.rawBytes ?? 0), 0),
			hls: list.reduce((n, m) => n + (m.hlsBytes ?? 0), 0),
			count: list.length,
			originals: list.filter(deletable).map((m) => m.id)
		};
	}

	const all = $derived(totals(media));
	const unmeasured = $derived(
		media.filter((m) => m.status === 'ready' && m.hlsBytes == null).length
	);

	// Per folder: everything inside it (subfolders too), biggest first.
	const folderTotals = $derived(
		new Map(
			folders.map((f) => {
				const ids = subtree(folders, f.id);
				return [f.id, totals(media.filter((m) => m.folderId && ids.has(m.folderId)))];
			})
		)
	);

	const childFolders = (parent: string | null) =>
		folders
			.filter((f) => (f.parentId ?? null) === parent)
			.sort(
				(a, b) =>
					(folderTotals.get(b.id)?.raw ?? 0) +
						(folderTotals.get(b.id)?.hls ?? 0) -
						((folderTotals.get(a.id)?.raw ?? 0) + (folderTotals.get(a.id)?.hls ?? 0)) ||
					byNumericName(a, b)
			);

	const mediaIn = (parent: string | null) =>
		media.filter((m) => (m.folderId ?? null) === parent).sort((a, b) => size(b) - size(a));

	async function deleteOriginals(ids: string[], what: string) {
		if (!ids.length) return;
		if (
			!confirm(
				`Delete ${ids.length === 1 ? 'the original' : `${ids.length} originals`} of ${what}? ` +
					"The movies keep playing, but can't be downloaded or re-processed any more."
			)
		)
			return;
		busy = true;
		try {
			await apiPost('/api/media/bulk', { ids, action: 'delete_original' });
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Storage · Library · whatevermp4</title></svelte:head>

<main class="container page">
	<div class="page-head">
		<div>
			<h1 class="section-title">Storage</h1>
			<p class="muted">
				Space used by each movie: the uploaded original plus the stream it was transcoded to.
			</p>
		</div>
		<a href={resolve('/library')} class="btn btn-outline"><ArrowLeft size={16} /> Library</a>
	</div>

	{#if error}<p class="error-text">{error}</p>{/if}

	<section class="stats">
		<div class="panel stat">
			<span class="label">Total</span>
			<b>{formatBytes(all.raw + all.hls)}</b>
			<span class="meta">{all.count} movies</span>
		</div>
		<div class="panel stat">
			<span class="label">Originals</span>
			<b>{formatBytes(all.raw)}</b>
			<button
				class="meta link"
				disabled={busy || !all.originals.length}
				onclick={() => deleteOriginals(all.originals, 'every ready movie')}
			>
				Delete all originals ({all.originals.length})
			</button>
		</div>
		<div class="panel stat">
			<span class="label">Streams</span>
			<b>{formatBytes(all.hls)}</b>
			{#if unmeasured}
				<span class="meta" title="The worker measures older movies at startup and hourly">
					{unmeasured} not measured yet
				</span>
			{/if}
		</div>
	</section>

	<section class="panel tree">
		<div class="row head">
			<span>Name</span>
			<span>Original</span>
			<span>Stream</span>
			<span></span>
		</div>
		{@render level(null, 0)}
	</section>
</main>

{#snippet level(parent: string | null, depth: number)}
	{#each childFolders(parent) as f (f.id)}
		{@const t = folderTotals.get(f.id)}
		<details open={depth === 0}>
			<summary class="row folder" style:--depth={depth}>
				<span class="name"
					><FolderIcon size={14} /> {f.name} <span class="meta">{t?.count ?? 0}</span></span
				>
				<span>{formatBytes(t?.raw ?? 0)}</span>
				<span>{formatBytes(t?.hls ?? 0)}</span>
				<span>
					{#if t?.originals.length}
						<button
							class="btn-icon"
							title="Delete the originals in this folder ({t.originals.length})"
							disabled={busy}
							onclick={(e) => {
								e.preventDefault();
								deleteOriginals(t.originals, `"${f.name}"`);
							}}
						>
							<FileX size={14} />
						</button>
					{/if}
				</span>
			</summary>
			{@render level(f.id, depth + 1)}
		</details>
	{/each}
	{#each mediaIn(parent) as m (m.id)}
		<div class="row" style:--depth={depth}>
			<span class="name" title={m.filename}>{m.filename}</span>
			<span>
				{#if m.rawDeleted}<span class="meta">deleted</span>
				{:else if m.rawBytes != null}{formatBytes(m.rawBytes)}
				{:else}<span class="meta">{m.status === 'ready' ? '…' : m.status}</span>{/if}
			</span>
			<span>{m.hlsBytes != null ? formatBytes(m.hlsBytes) : ''}</span>
			<span>
				{#if deletable(m)}
					<button
						class="btn-icon"
						title="Delete the original"
						disabled={busy}
						onclick={() => deleteOriginals([m.id], `"${m.filename}"`)}
					>
						<FileX size={14} />
					</button>
				{/if}
			</span>
		</div>
	{/each}
	{#if parent === null && !folders.length && !media.length}
		<p class="muted empty"><HardDrive size={16} /> Nothing stored yet.</p>
	{/if}
{/snippet}

<style>
	.stats {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(12rem, 1fr));
		gap: 1rem;
		margin-bottom: 1.5rem;
	}

	.stat {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.stat b {
		font-size: 1.5rem;
		font-weight: 800;
	}

	.link {
		align-self: flex-start;
		color: var(--color-accent);
	}

	.link:hover:not(:disabled) {
		text-decoration: underline;
	}

	.link:disabled {
		color: var(--color-gray);
	}

	.tree {
		padding: 0.5rem 0;
		font-size: 13px;
	}

	.row {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 6.5rem 6.5rem 2.5rem;
		align-items: center;
		gap: 0.75rem;
		min-height: 2.25rem;
		padding: 0 1.25rem 0 calc(1.25rem + var(--depth, 0) * 1.25rem);
		font-variant-numeric: tabular-nums;
	}

	.row > span:nth-child(2),
	.row > span:nth-child(3) {
		text-align: right;
	}

	.row.head {
		border-bottom: 1px solid var(--color-border);
		font-size: 11px;
		font-weight: 700;
		text-transform: uppercase;
		color: var(--color-gray);
	}

	.row:not(.head):hover {
		background-color: color-mix(in srgb, var(--color-bg), transparent 50%);
	}

	.folder {
		cursor: pointer;
		font-weight: 700;
		list-style: none;
	}

	.folder::-webkit-details-marker {
		display: none;
	}

	.name {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.row .btn-icon {
		min-width: 28px;
		height: 28px;
		color: var(--color-gray);
	}

	.empty {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 1rem 1.25rem;
	}
</style>
