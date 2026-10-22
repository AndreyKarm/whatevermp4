<script lang="ts">
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

<section class="bg-gray-800 rounded p-4">
	<h2 class="font-bold mb-3">Torrents</h2>
	<form
		class="flex gap-2 mb-2"
		onsubmit={(e) => {
			e.preventDefault();
			if (magnet.trim()) add({ magnet: magnet.trim() });
		}}
	>
		<input
			type="text"
			placeholder="magnet:?xt=…"
			bind:value={magnet}
			class="flex-1 bg-gray-700 p-2 rounded text-sm"
		/>
		<button class="bg-blue-600 px-3 rounded text-sm disabled:opacity-50" disabled={busy}>Add</button
		>
	</form>
	<label class="block text-xs text-gray-400 mb-3">
		or a .torrent file
		<input type="file" accept=".torrent" onchange={addFile} disabled={busy} class="text-xs" />
	</label>
	{#if error}<p class="text-xs text-red-400 mb-2">{error}</p>{/if}

	<ul class="space-y-3 text-sm">
		{#each torrents as t (t.id)}
			<li>
				<div class="flex justify-between gap-2">
					<span class="truncate">{t.name}</span>
					<span class="text-xs text-gray-400 shrink-0">{t.status}</span>
				</div>
				{#if t.status === 'downloading' || t.status === 'queued'}
					<div class="flex items-center gap-2 mt-1">
						<div class="flex-1 h-1 bg-gray-700 rounded">
							<div class="h-1 bg-blue-500 rounded" style:width="{t.progress * 100}%"></div>
						</div>
						<span class="text-xs text-gray-400">
							{size(t.bytesDone)} / {size(t.bytesTotal)}
						</span>
						<button class="text-xs text-red-400" onclick={() => cancel(t.id)}>Cancel</button>
					</div>
				{/if}
				{#if t.error}<p class="text-xs text-red-400">{t.error}</p>{/if}
			</li>
		{:else}
			<li class="text-gray-500 text-xs">No torrents yet.</li>
		{/each}
	</ul>
</section>
