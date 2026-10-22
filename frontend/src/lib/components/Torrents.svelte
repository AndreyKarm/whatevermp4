<script lang="ts">
	import { X } from '@lucide/svelte';
	import { apiFetch, apiPost } from '#lib/api.ts';
	import type { TorrentItem } from '#lib/types.ts';

	let { torrents, onchange }: { torrents: TorrentItem[]; onchange?: () => void } = $props();

	let magnet = $state('');
	let busy = $state(false);
	let error = $state<string | null>(null);

	async function add(body: { magnet?: string; metainfo?: string }) {
		busy = true;
		error = null;
		try {
			await apiPost('/api/torrents', body);
			magnet = '';
			onchange?.();
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		} finally {
			busy = false;
		}
	}

	function addFile(e: Event) {
		const input = e.target as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;
		const reader = new FileReader();
		reader.onload = () => {
			// data:application/x-bittorrent;base64,<payload>
			const metainfo = String(reader.result).split(',', 2)[1] ?? '';
			add({ metainfo });
			input.value = '';
		};
		reader.readAsDataURL(file);
	}

	async function cancel(id: string) {
		try {
			await apiFetch(`/api/torrents/${id}`, { method: 'DELETE' });
			onchange?.();
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

<section class="panel">
	<h2 class="panel-title">Torrents</h2>
	<form
		class="mb-3 flex gap-2"
		onsubmit={(e) => {
			e.preventDefault();
			if (magnet.trim()) add({ magnet: magnet.trim() });
		}}
	>
		<input
			type="text"
			placeholder="magnet:?xt=…"
			bind:value={magnet}
			class="input min-w-0 flex-1"
		/>
		<button class="btn btn-primary" disabled={busy}>Add</button>
	</form>
	<label class="mb-4 flex flex-col gap-2">
		<span class="label">or a .torrent file</span>
		<input type="file" accept=".torrent" onchange={addFile} disabled={busy} class="input" />
	</label>
	{#if error}<p class="mb-2 text-xs text-accent">{error}</p>{/if}

	<ul class="space-y-4 text-sm">
		{#each torrents as t (t.id)}
			<li>
				<div class="flex justify-between gap-2">
					<span class="truncate text-xs font-bold uppercase" title={t.name}>{t.name}</span>
					<span
						class="meta shrink-0"
						class:text-primary={t.status === 'done'}
						class:text-accent={t.status === 'failed'}
					>
						{t.status}
					</span>
				</div>
				{#if t.status === 'downloading' || t.status === 'queued'}
					<div class="mt-2 flex items-center gap-2">
						<div class="progress flex-1">
							<div style:width="{t.progress * 100}%"></div>
						</div>
						<span class="meta normal-case">{size(t.bytesDone)} / {size(t.bytesTotal)}</span>
						<button
							class="btn-icon delete text-gray-500"
							title="Cancel"
							onclick={() => cancel(t.id)}
						>
							<X size={14} />
						</button>
					</div>
				{/if}
				{#if t.error}<p class="text-xs text-accent">{t.error}</p>{/if}
			</li>
		{:else}
			<li class="text-xs text-muted">No torrents yet.</li>
		{/each}
	</ul>
</section>
