<script lang="ts">
	import { apiPost } from '#lib/api.ts';

	let {
		folderId = null,
		onadded
	}: {
		folderId?: string | null;
		/** Called after the import was queued. */
		onadded?: () => void;
	} = $props();

	let url = $state('');
	let name = $state('');
	let referer = $state('');
	let showReferer = $state(false);
	let busy = $state(false);
	let error = $state<string | null>(null);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!url.trim()) return;
		busy = true;
		error = null;
		try {
			await apiPost('/api/imports', {
				url: url.trim(),
				name: name.trim(),
				referer: referer.trim(),
				folderId
			});
			url = name = referer = '';
			onadded?.();
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		} finally {
			busy = false;
		}
	}
</script>

<form class="tab" onsubmit={submit}>
	<label class="field">
		<span class="label">.m3u8 URL</span>
		<input
			type="url"
			required
			placeholder="https://…/manifest.m3u8"
			bind:value={url}
			class="input"
		/>
	</label>
	<label class="field">
		<span class="label">Title (optional)</span>
		<input type="text" placeholder="Show S01E01" bind:value={name} class="input" />
	</label>
	{#if showReferer}
		<label class="field">
			<span class="label">Referer (if the site requires it)</span>
			<input type="url" placeholder="https://…" bind:value={referer} class="input" />
		</label>
	{:else}
		<button type="button" class="meta add-referer" onclick={() => (showReferer = true)}>
			+ Referer header
		</button>
	{/if}
	<p class="muted">
		The stream is copied without re-encoding, so it downloads at the source's speed.
	</p>
	{#if error}<p class="error-text">{error}</p>{/if}
	<button class="btn btn-primary" disabled={busy}>{busy ? 'Adding…' : 'Import'}</button>
</form>

<style>
	.tab {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.add-referer {
		align-self: flex-start;
	}

	.add-referer:hover {
		color: var(--color-primary);
	}
</style>
