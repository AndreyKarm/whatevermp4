<script lang="ts">
	import { apiPost } from '#lib/api.ts';

	let { onimported }: { onimported?: () => void } = $props();

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
				referer: referer.trim()
			});
			url = name = referer = '';
			onimported?.();
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		} finally {
			busy = false;
		}
	}
</script>

<section class="panel">
	<h2 class="panel-title">Import from link</h2>
	<form class="flex flex-col gap-3" onsubmit={submit}>
		<label class="flex flex-col gap-2">
			<span class="label">.m3u8 URL</span>
			<input
				type="url"
				required
				placeholder="https://…/manifest.m3u8"
				bind:value={url}
				class="input"
			/>
		</label>
		<label class="flex flex-col gap-2">
			<span class="label">Title (optional)</span>
			<input type="text" placeholder="Show S01E01" bind:value={name} class="input" />
		</label>
		{#if showReferer}
			<label class="flex flex-col gap-2">
				<span class="label">Referer (if the site requires it)</span>
				<input type="url" placeholder="https://…" bind:value={referer} class="input" />
			</label>
		{:else}
			<button
				type="button"
				class="meta self-start hover:text-primary"
				onclick={() => (showReferer = true)}
			>
				+ Referer header
			</button>
		{/if}
		<button class="btn btn-primary" disabled={busy}>{busy ? 'Adding…' : 'Import'}</button>
	</form>
	{#if error}<p class="mt-2 text-xs text-accent">{error}</p>{/if}
</section>
