<script lang="ts">
	import { Play, RefreshCw, Trash2 } from '@lucide/svelte';
	import { apiFetch } from '#lib/api.ts';
	import { folderOptions } from '#lib/folders.ts';
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

<Modal open={!!item} title="Edit movie" width="30rem" {onclose}>
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
			<div class="actions">
				<button type="button" class="btn btn-outline" onclick={onclose}>Cancel</button>
				<button class="btn btn-primary" disabled={busy}>Save</button>
			</div>
		</form>

		<hr />

		<div class="more">
			{#if item.status === 'ready'}
				<button class="btn btn-outline" disabled={busy} onclick={() => item && onwatch(item)}>
					<Play size={14} /> Watch together
				</button>
			{/if}
			{#if item.status === 'ready' || item.status === 'failed'}
				<button
					class="btn btn-outline"
					disabled={busy}
					title="Transcode again from the original file"
					onclick={reprocess}
				>
					<RefreshCw size={14} /> Re-process
				</button>
			{/if}
			<button class="btn btn-danger" disabled={busy} onclick={remove}>
				<Trash2 size={14} />
				{confirmDelete ? 'Click again to delete' : 'Delete'}
			</button>
		</div>
		{#if item.error}<p class="error-text">Last error: {item.error}</p>{/if}
		{#if error}<p class="error-text">{error}</p>{/if}
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

	.actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
	}

	hr {
		width: 100%;
		margin: 0;
		border: none;
		border-top: 1px solid var(--color-border);
	}

	.more {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}
</style>
