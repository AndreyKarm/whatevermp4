<script lang="ts">
	import { Trash2 } from '@lucide/svelte';
	import { apiFetch, apiPost } from '#lib/api.ts';
	import { folderOptions, pathLabel, subtree } from '#lib/folders.ts';
	import type { Folder } from '#lib/types.ts';
	import Modal from './Modal.svelte';

	let {
		open,
		folder,
		parentId,
		folders,
		onclose,
		ondeleted
	}: {
		open: boolean;
		/** null = create a new folder inside parentId */
		folder: Folder | null;
		parentId: string | null;
		folders: Folder[];
		onclose: () => void;
		/** after deleting, e.g. to leave the deleted folder */
		ondeleted?: (parentId: string | null) => void;
	} = $props();

	let name = $state('');
	let parent = $state('');
	let busy = $state(false);
	let error = $state<string | null>(null);
	let confirmDelete = $state(false);

	// Reset the form each time the modal opens.
	let wasOpen = false;
	$effect.pre(() => {
		if (open && !wasOpen) {
			name = folder?.name ?? '';
			parent = (folder ? folder.parentId : parentId) ?? '';
			error = null;
			confirmDelete = false;
		}
		wasOpen = open;
	});

	// A folder can't move into itself or its own subfolders.
	const parentChoices = $derived(
		folderOptions(folders, folder ? subtree(folders, folder.id) : new Set())
	);
	const parentLabel = $derived(parent ? pathLabel(folders, parent) : 'the top level');

	async function run(action: () => Promise<unknown>) {
		busy = true;
		error = null;
		try {
			await action();
			return true;
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
			return false;
		} finally {
			busy = false;
		}
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		if (!name.trim()) return;
		const ok = await run(() =>
			folder
				? apiFetch(`/api/folders/${folder.id}`, {
						method: 'PATCH',
						body: JSON.stringify({ name: name.trim(), parentId: parent || null })
					})
				: apiPost('/api/folders', { name: name.trim(), parentId: parent || null })
		);
		if (ok) onclose();
	}

	async function remove() {
		if (!folder) return;
		if (!confirmDelete) {
			confirmDelete = true;
			return;
		}
		const id = folder.id;
		const up = folder.parentId ?? null;
		const ok = await run(() => apiFetch(`/api/folders/${id}`, { method: 'DELETE' }));
		if (ok) {
			onclose();
			ondeleted?.(up);
		}
	}
</script>

<Modal {open} title={folder ? 'Edit folder' : 'New folder'} width="28rem" {onclose}>
	<form class="form" onsubmit={save}>
		<label class="field">
			<span class="label">Name</span>
			<input
				class="input"
				maxlength="100"
				required
				placeholder="Show name, Season 1, …"
				bind:value={name}
			/>
		</label>
		<label class="field">
			<span class="label">Inside</span>
			<select class="input" bind:value={parent}>
				<option value="">— Top level —</option>
				{#each parentChoices as o (o.id)}
					<option value={o.id}>{o.label}</option>
				{/each}
			</select>
		</label>
		<div class="actions">
			{#if folder}
				<button type="button" class="btn btn-danger" disabled={busy} onclick={remove}>
					<Trash2 size={14} />
					{confirmDelete ? 'Click again to delete' : 'Delete'}
				</button>
			{/if}
			<span class="spacer"></span>
			<button type="button" class="btn btn-outline" onclick={onclose}>Cancel</button>
			<button class="btn btn-primary" disabled={busy}>{folder ? 'Save' : 'Create'}</button>
		</div>
		{#if confirmDelete}
			<p class="muted">Its folders and movies move up to {parentLabel}; nothing is deleted.</p>
		{/if}
		{#if error}<p class="error-text">{error}</p>{/if}
	</form>
</Modal>

<style>
	.form {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.75rem;
	}

	.spacer {
		flex: 1;
	}
</style>
