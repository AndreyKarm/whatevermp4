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

<div class="p-4 rounded bg-gray-800 text-white w-full md:w-80">
	<label class="block">
		<span class="block mb-2 text-sm font-semibold">
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
			class="block w-full text-sm"
			disabled={uploader.progress !== null}
		/>
	</label>
	{#if uploader.progress !== null}
		<div class="mt-2 h-1 bg-gray-700 rounded">
			<div class="h-1 bg-blue-500 rounded" style:width="{uploader.progress * 100}%"></div>
		</div>
	{/if}
	{#if error}<p class="mt-2 text-xs text-red-400">{error}</p>{/if}
</div>
