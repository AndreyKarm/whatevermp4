<script lang="ts">
	import { S3Uploader } from '#lib/upload.svelte.ts';

	let { onuploaded }: { onuploaded?: () => void } = $props();

	const uploader = new S3Uploader();
	let error = $state<string | null>(null);

	async function handleFile(e: Event) {
		const input = e.target as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;

		error = null;
		try {
			await uploader.upload(file);
			onuploaded?.();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Upload failed';
		} finally {
			input.value = '';
		}
	}
</script>

<div class="panel w-full p-4 md:w-80">
	<label class="flex flex-col gap-2">
		<span class="label">
			{#if uploader.progress !== null}
				Uploading… {Math.round(uploader.progress * 100)}%
			{:else}
				Upload a video
			{/if}
		</span>
		<input
			type="file"
			accept="video/*,.mkv"
			onchange={handleFile}
			class="input w-full"
			disabled={uploader.progress !== null}
		/>
	</label>
	{#if uploader.progress !== null}
		<div class="progress mt-3">
			<div style:width="{uploader.progress * 100}%"></div>
		</div>
	{/if}
	{#if error}<p class="mt-2 text-xs text-accent">{error}</p>{/if}
</div>
