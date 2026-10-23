<script lang="ts">
	import { uploader } from '#lib/upload.svelte.ts';

	let {
		folderId = null,
		onstarted
	}: {
		folderId?: string | null;
		/** Called once the upload has started; progress continues in the library. */
		onstarted?: () => void;
	} = $props();

	function handleFiles(e: Event) {
		const input = e.target as HTMLInputElement;
		if (!input.files?.length) return;
		// Errors are kept on `uploader.error` and shown in the downloads list.
		uploader.enqueue(input.files, folderId);
		input.value = '';
		onstarted?.();
	}
</script>

<div class="tab">
	<label class="field">
		<span class="label">Video files</span>
		<input type="file" accept="video/*,.mkv" multiple onchange={handleFiles} class="input" />
	</label>
	<p class="muted">
		{#if uploader.busy}
			"{uploader.filename}" is uploading; new files wait their turn.
		{:else}
			Uploads go straight to storage in 64 MB parts, one file at a time. You can also drop files
			onto the library. Progress shows there.
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
