<script lang="ts">
	import { apiPost } from '#lib/api.ts';

	/** Called after the torrent was queued. */
	let { onadded }: { onadded?: () => void } = $props();

	let magnet = $state('');
	let busy = $state(false);
	let error = $state<string | null>(null);

	async function add(body: { magnet?: string; metainfo?: string }) {
		busy = true;
		error = null;
		try {
			await apiPost('/api/torrents', body);
			magnet = '';
			onadded?.();
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
</script>

<div class="tab">
	<form
		class="field"
		onsubmit={(e) => {
			e.preventDefault();
			if (magnet.trim()) add({ magnet: magnet.trim() });
		}}
	>
		<label class="label" for="magnet">Magnet link</label>
		<div class="row">
			<input id="magnet" type="text" placeholder="magnet:?xt=…" bind:value={magnet} class="input" />
			<button class="btn btn-primary" disabled={busy || !magnet.trim()}>Add</button>
		</div>
	</form>
	<label class="field">
		<span class="label">or a .torrent file</span>
		<input type="file" accept=".torrent" onchange={addFile} disabled={busy} class="input" />
	</label>
	<p class="muted">Only the video files are downloaded. Each becomes its own library entry.</p>
	{#if error}<p class="error-text">{error}</p>{/if}
</div>

<style>
	.tab {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.row {
		display: flex;
		gap: 0.5rem;
	}

	.row .input {
		min-width: 0;
		flex: 1;
	}
</style>
