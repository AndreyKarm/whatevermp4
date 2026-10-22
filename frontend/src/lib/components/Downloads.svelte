<script lang="ts">
	import { X } from '@lucide/svelte';
	import { apiFetch } from '#lib/api.ts';
	import { uploader } from '#lib/upload.svelte.ts';
	import type { TorrentItem } from '#lib/types.ts';

	/** In-flight work that doesn't have a library card yet: the browser upload and torrents. */
	let { torrents }: { torrents: TorrentItem[] } = $props();

	let error = $state<string | null>(null);

	// Finished torrents already show up as media; keep failures visible.
	const active = $derived(torrents.filter((t) => t.status !== 'done' && t.status !== 'cancelled'));
	const visible = $derived(uploader.busy || !!uploader.error || active.length > 0);

	async function cancel(id: string) {
		try {
			await apiFetch(`/api/torrents/${id}`, { method: 'DELETE' });
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		}
	}

	function size(n: number) {
		if (n < 1024 ** 2) return `${(n / 1024).toFixed(0)} KB`;
		if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(1)} MB`;
		return `${(n / 1024 ** 3).toFixed(2)} GB`;
	}
</script>

{#if visible}
	<section class="panel downloads">
		<h2 class="panel-title">In progress</h2>
		<ul>
			{#if uploader.busy}
				<li>
					<div class="row">
						<span class="name">{uploader.filename}</span>
						<span class="meta status-processing">
							uploading {Math.round((uploader.progress ?? 0) * 100)}%
						</span>
					</div>
					<div class="row">
						<div class="progress grow">
							<div style:width="{(uploader.progress ?? 0) * 100}%"></div>
						</div>
						<span class="meta">
							{size((uploader.progress ?? 0) * uploader.size)} / {size(uploader.size)}
						</span>
					</div>
				</li>
			{:else if uploader.error}
				<li>
					<div class="row">
						<span class="name">{uploader.filename}</span>
						<button class="meta" onclick={() => (uploader.error = null)}>dismiss</button>
					</div>
					<p class="error-text">Upload failed: {uploader.error}</p>
				</li>
			{/if}
			{#each active as t (t.id)}
				<li>
					<div class="row">
						<span class="name" title={t.name}>{t.name}</span>
						<span class="meta status-{t.status === 'failed' ? 'failed' : 'processing'}">
							{t.status}
						</span>
						{#if t.status === 'queued' || t.status === 'downloading' || t.status === 'failed'}
							<button
								class="btn-icon"
								title={t.status === 'failed' ? 'Dismiss' : 'Cancel'}
								onclick={() => cancel(t.id)}
							>
								<X size={14} />
							</button>
						{/if}
					</div>
					{#if t.status === 'downloading'}
						<div class="row">
							<div class="progress grow"><div style:width="{t.progress * 100}%"></div></div>
							<span class="meta">{size(t.bytesDone)} / {size(t.bytesTotal)}</span>
						</div>
					{/if}
					{#if t.error}<p class="error-text">{t.error}</p>{/if}
				</li>
			{/each}
		</ul>
		{#if error}<p class="error-text">{error}</p>{/if}
	</section>
{/if}

<style>
	.downloads {
		margin-bottom: 2rem;
	}

	ul {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		margin-top: 1rem;
	}

	li {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	.name {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 12px;
		font-weight: 700;
		text-transform: uppercase;
	}

	.grow {
		flex: 1;
	}
</style>
