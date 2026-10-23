<script lang="ts">
	import { CircleX, FileX, Play, RefreshCw, Trash2 } from '@lucide/svelte';
	import { apiFetch } from '#lib/api.ts';
	import { formatBytes } from '#lib/bytes.ts';
	import { folderOptions } from '#lib/folders.ts';
	import { uploader } from '#lib/upload.svelte.ts';
	import type { Folder, MediaItem } from '#lib/types.ts';
	import Modal from './Modal.svelte';

	let {
		item,
		folders,
		onclose,
		onwatch
	}: {
		/** null = closed */
		item: MediaItem | null;
		folders: Folder[];
		onclose: () => void;
		onwatch: (item: MediaItem) => void;
	} = $props();

	const folderChoices = $derived(folderOptions(folders));

	const sizeInfo = $derived.by(() => {
		if (!item) return '';
		const parts = [];
		if (item.rawDeleted) parts.push('Original deleted');
		else if (item.rawBytes != null) parts.push(`Original ${formatBytes(item.rawBytes)}`);
		if (item.hlsBytes != null) parts.push(`Stream ${formatBytes(item.hlsBytes)}`);
		return parts.join(' · ');
	});

	// Uploading, downloading or processing: the only action is to stop it.
	const working = $derived(
		item?.status === 'uploading' || item?.status === 'downloading' || item?.status === 'processing'
	);
	// A re-process keeps the previous version when cancelled; anything else is removed.
	const reprocessing = $derived(item?.status === 'processing' && !!item.hlsKey);
	const uploadingHere = $derived(!!item && uploader.busy && uploader.mediaId === item.id);
	const progress = $derived(uploadingHere ? uploader.progress : (item?.progress ?? null));
	const cancelLabel = $derived(
		item?.status === 'uploading'
			? 'Cancel upload'
			: item?.status === 'downloading'
				? 'Cancel download'
				: reprocessing
					? 'Cancel re-process'
					: 'Cancel processing'
	);

	let filename = $state('');
	let folderId = $state('');
	// number inputs bind as number, or null when empty
	let season = $state<number | null>(null);
	let episode = $state<number | null>(null);
	let busy = $state(false);
	let error = $state<string | null>(null);
	let confirmDelete = $state(false);

	// Reset the form whenever a different item is opened.
	let openedId = '';
	$effect.pre(() => {
		if (item && item.id !== openedId) {
			openedId = item.id;
			filename = item.filename;
			folderId = item.folderId ?? '';
			season = item.season ?? null;
			episode = item.episode ?? null;
			error = null;
			confirmDelete = false;
		} else if (!item) {
			openedId = '';
		}
	});

	function num(v: number | null): number | null {
		return typeof v === 'number' && Number.isInteger(v) && v >= 0 ? v : null;
	}

	async function run(action: () => Promise<unknown>, close = true) {
		busy = true;
		error = null;
		try {
			await action();
			if (close) onclose();
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		} finally {
			busy = false;
		}
	}

	function save(e: SubmitEvent) {
		e.preventDefault();
		if (!item || !filename.trim()) return;
		const id = item.id;
		run(() =>
			apiFetch(`/api/media/${id}`, {
				method: 'PATCH',
				body: JSON.stringify({
					filename: filename.trim(),
					season: num(season),
					episode: num(episode),
					folderId: folderId || null
				})
			})
		);
	}

	function reprocess() {
		if (!item) return;
		const id = item.id;
		run(() => apiFetch(`/api/media/${id}/reprocess`, { method: 'POST' }));
	}

	function deleteOriginal() {
		if (!item) return;
		if (
			!confirm(
				'Delete the original file? The movie still plays, but it can no longer be downloaded or re-processed.'
			)
		)
			return;
		const id = item.id;
		run(() => apiFetch(`/api/media/${id}/delete-original`, { method: 'POST' }), false);
	}

	function cancelWork() {
		if (!item) return;
		if (
			!reprocessing &&
			!confirm(`Stop and remove "${item.filename}"? Everything stored for it is deleted.`)
		)
			return;
		if (uploadingHere) {
			// this browser is sending it: stop the parts too (the row goes with them)
			uploader.cancel();
			onclose();
			return;
		}
		const id = item.id;
		// a cancelled re-process stays, back to its previous version
		run(() => apiFetch(`/api/media/${id}/cancel`, { method: 'POST' }), !reprocessing);
	}

	function remove() {
		if (!item) return;
		if (!confirmDelete) {
			confirmDelete = true;
			return;
		}
		const id = item.id;
		run(() => apiFetch(`/api/media/${id}`, { method: 'DELETE' }));
	}
</script>

<Modal open={!!item} title="Edit movie" width="32rem" {onclose}>
	{#if item}
		<form class="form" onsubmit={save}>
			<label class="field">
				<span class="label">Title</span>
				<input class="input" maxlength="255" required bind:value={filename} />
			</label>
			<label class="field">
				<span class="label">Folder</span>
				<select class="input" bind:value={folderId}>
					<option value="">— Top level —</option>
					{#each folderChoices as o (o.id)}
						<option value={o.id}>{o.label}</option>
					{/each}
				</select>
			</label>
			<div class="row">
				<label class="field">
					<span class="label">Season</span>
					<input class="input" type="number" min="0" placeholder="—" bind:value={season} />
				</label>
				<label class="field">
					<span class="label">Episode</span>
					<input class="input" type="number" min="0" placeholder="—" bind:value={episode} />
				</label>
			</div>
			<p class="muted">Season and episode set the order when the library is sorted by episode.</p>

			<section class="state" aria-label="Status and actions">
				<div class="status">
					<span class="meta status-{item.status}">
						{item.status}{#if working && progress != null}&nbsp;{Math.round(progress * 100)}%{/if}
					</span>
					{#if sizeInfo}<span class="meta sizes">{sizeInfo}</span>{/if}
				</div>
				{#if working}
					<div class="progress" class:indeterminate={progress == null}>
						<div style:width={progress != null ? `${progress * 100}%` : undefined}></div>
					</div>
				{/if}
				{#if item.error}<p class="error-text">Last error: {item.error}</p>{/if}

				<div class="actions">
					{#if working}
						<button type="button" class="btn btn-danger" disabled={busy} onclick={cancelWork}>
							<CircleX size={14} />
							{cancelLabel}
						</button>
					{:else}
						{#if item.status === 'ready'}
							<button
								type="button"
								class="btn btn-outline"
								disabled={busy}
								onclick={() => item && onwatch(item)}
							>
								<Play size={14} /> Watch together
							</button>
						{/if}
						{#if !item.rawDeleted}
							<button
								type="button"
								class="btn btn-outline"
								disabled={busy}
								title="Transcode again from the original file"
								onclick={reprocess}
							>
								<RefreshCw size={14} /> Re-process
							</button>
						{/if}
						{#if item.status === 'ready' && !item.rawDeleted}
							<button
								type="button"
								class="btn btn-outline"
								disabled={busy}
								title="Free the space of the uploaded original; the movie keeps playing"
								onclick={deleteOriginal}
							>
								<FileX size={14} /> Delete original
							</button>
						{/if}
					{/if}
				</div>
			</section>

			{#if error}<p class="error-text">{error}</p>{/if}

			<footer class="footer">
				<button type="button" class="btn btn-danger" disabled={busy} onclick={remove}>
					<Trash2 size={14} />
					{confirmDelete ? 'Click again to delete' : 'Delete'}
				</button>
				<span class="spacer"></span>
				<button type="button" class="btn btn-outline" onclick={onclose}>Close</button>
				<button class="btn btn-primary" disabled={busy}>Save</button>
			</footer>
		</form>
	{/if}
</Modal>

<style>
	.form {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.row {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 1rem;
	}

	/* status, progress and what can be done with the movie, between two rules */
	.state {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		padding: 1rem 0;
		border-top: 1px solid var(--color-border);
		border-bottom: 1px solid var(--color-border);
	}

	.status {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.25rem 1rem;
	}

	.status .meta {
		font-size: 11px;
	}

	.sizes {
		text-transform: none;
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

	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}

	.actions:empty {
		display: none;
	}

	.actions .btn {
		flex: 1 1 0;
		padding: 0.55rem 0.75rem;
		font-size: 12px;
	}

	.footer {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.75rem;
	}

	.spacer {
		flex: 1;
	}
</style>
