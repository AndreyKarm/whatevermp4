<script lang="ts">
	import { uploader } from '#lib/upload.svelte.ts';

	/** Called once the upload has started; progress continues in the library. */
	let { onstarted }: { onstarted?: () => void } = $props();

	function handleFile(e: Event) {
		const input = e.target as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;
		// Errors are kept on `uploader.error` and shown in the downloads list.
		uploader.upload(file).catch(() => {});
		input.value = '';
		onstarted?.();
	}
</script>

<div class="tab">
	<label class="field">
		<span class="label">Video file</span>
		<input
			type="file"
			accept="video/*,.mkv"
			onchange={handleFile}
			class="input"
			disabled={uploader.busy}
		/>
	</label>
	<p class="muted">
		{#if uploader.busy}
			Wait for "{uploader.filename}" to finish uploading first.
		{:else}
			Uploads go straight to storage in 64 MB parts. Progress shows in the library.
		{/if}
	</p>
</div>

<style>
	.tab {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}
</style>
