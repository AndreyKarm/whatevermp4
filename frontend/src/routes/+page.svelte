<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { PUBLIC_API_URL } from '$app/env/public';
	import { apiFetch, apiPost } from '#lib/api.ts';
	import type { MediaItem, RoomItem, TorrentItem } from '#lib/types.ts';
	import Uploader from '#lib/components/Uploader.svelte';
	import Torrents from '#lib/components/Torrents.svelte';

	let media: MediaItem[] = $state([]);
	let rooms: RoomItem[] = $state([]);
	let torrents: TorrentItem[] = $state([]);
	let error = $state<string | null>(null);

	async function load() {
		try {
			[media, rooms, torrents] = await Promise.all([
				apiFetch<MediaItem[]>('/api/media'),
				apiFetch<RoomItem[]>('/api/rooms'),
				apiFetch<TorrentItem[]>('/api/torrents')
			]);
			error = null;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}

	onMount(() => {
		load();
		// Poll for upload/processing/torrent progress
		const interval = setInterval(load, 5000);
		return () => clearInterval(interval);
	});

	async function watch(item: MediaItem) {
		try {
			const { id } = await apiPost<{ id: string }>('/api/rooms', {
				name: item.filename,
				mediaId: item.id
			});
			goto(resolve('/rooms/[id]', { id }));
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}

	async function remove(item: MediaItem) {
		if (!confirm(`Delete "${item.filename}"?`)) return;
		try {
			await apiFetch(`/api/media/${item.id}`, { method: 'DELETE' });
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}
</script>

<main class="p-8 space-y-8">
	<div class="flex flex-col md:flex-row justify-between md:items-center gap-4">
		<h1 class="text-3xl font-bold">Library</h1>
		<Uploader onuploaded={load} />
	</div>

	{#if error}<p class="text-red-400 text-sm">{error}</p>{/if}

	<div class="grid grid-cols-1 lg:grid-cols-[1fr_20rem] gap-8">
		<div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-6 content-start">
			{#each media as item (item.id)}
				<div class="bg-gray-800 rounded overflow-hidden">
					<div class="aspect-video bg-gray-700 flex items-center justify-center">
						{#if item.status === 'ready' && item.posterKey && item.playbackUrl}
							<img
								src="{PUBLIC_API_URL}{item.playbackUrl}/poster.jpg"
								alt=""
								class="w-full h-full object-cover"
							/>
						{:else}
							<span class="text-sm uppercase">{item.status}</span>
						{/if}
					</div>
					<div class="p-4">
						<h3 class="font-semibold truncate" title={item.filename}>{item.filename}</h3>
						<p class="text-xs text-gray-400">
							{item.status}{#if item.season != null}
								· S{item.season}E{item.episode}{/if} · by {item.uploadedBy}
						</p>
						{#if item.error}<p class="text-xs text-red-400 truncate" title={item.error}>
								{item.error}
							</p>{/if}
						<div class="flex gap-3 mt-2 text-sm">
							{#if item.status === 'ready'}
								<button class="text-blue-400" onclick={() => watch(item)}>Watch together</button>
							{/if}
							<button class="text-red-400 ml-auto" onclick={() => remove(item)}>Delete</button>
						</div>
					</div>
				</div>
			{:else}
				<p class="text-gray-500">Nothing here yet. Upload a video or add a torrent.</p>
			{/each}
		</div>

		<div class="space-y-6">
			<section class="bg-gray-800 rounded p-4">
				<h2 class="font-bold mb-3">Rooms</h2>
				<ul class="space-y-2 text-sm">
					{#each rooms as r (r.id)}
						<li>
							<a href={resolve('/rooms/[id]', { id: r.id })} class="text-blue-400">{r.name}</a>
							<span class="text-xs text-gray-400">by {r.ownerName}</span>
						</li>
					{:else}
						<li class="text-gray-500 text-xs">No rooms yet.</li>
					{/each}
				</ul>
			</section>
			<Torrents {torrents} onchange={load} />
		</div>
	</div>
</main>
