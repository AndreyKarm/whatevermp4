<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { PUBLIC_API_URL } from '$app/env/public';
	import { Film, Play, Trash2 } from '@lucide/svelte';
	import { apiFetch, apiPost } from '#lib/api.ts';
	import type { MediaItem, MediaStatus, RoomItem, TorrentItem } from '#lib/types.ts';
	import Uploader from '#lib/components/Uploader.svelte';
	import Torrents from '#lib/components/Torrents.svelte';

	let media: MediaItem[] = $state([]);
	let rooms: RoomItem[] = $state([]);
	let torrents: TorrentItem[] = $state([]);
	let error = $state<string | null>(null);

	const statusColor: Record<MediaStatus, string> = {
		ready: 'text-primary',
		processing: 'text-warn',
		uploading: 'text-gray-400',
		failed: 'text-accent'
	};

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

<main class="mx-auto w-full max-w-[75em] space-y-8 px-8 py-8">
	<div class="flex flex-col justify-between gap-4 md:flex-row md:items-end">
		<div>
			<h1 class="section-title">Library</h1>
			<p class="text-sm text-muted">Upload a video or add a torrent, then watch it together.</p>
		</div>
		<Uploader onuploaded={load} />
	</div>

	{#if error}<p class="text-sm text-accent">{error}</p>{/if}

	<div class="grid grid-cols-1 gap-8 lg:grid-cols-[1fr_20rem]">
		<div class="grid grid-cols-2 content-start gap-6 md:grid-cols-3 xl:grid-cols-4">
			{#each media as item (item.id)}
				<article class="group flex min-w-0 flex-col gap-3">
					<div class="relative isolate aspect-video overflow-hidden rounded-sm bg-secondary">
						{#if item.status === 'ready' && item.posterKey && item.playbackUrl}
							<img
								src="{PUBLIC_API_URL}{item.playbackUrl}/poster.jpg"
								alt=""
								draggable="false"
								class="h-full w-full object-cover transition-transform duration-300 group-hover:-translate-y-1 group-hover:scale-105"
							/>
						{:else}
							<div class="flex h-full items-center justify-center text-muted">
								<Film size={32} />
							</div>
						{/if}
						{#if item.season != null}
							<span class="badge absolute top-2 right-2">S{item.season}E{item.episode}</span>
						{/if}
					</div>

					<div class="flex min-w-0 flex-1 flex-col justify-between gap-2">
						<h3
							class="line-clamp-2 text-xs leading-snug font-bold break-all uppercase"
							title={item.filename}
						>
							{item.filename}
						</h3>
						{#if item.error}
							<p class="truncate text-xs text-accent" title={item.error}>{item.error}</p>
						{/if}
						<div class="flex items-center justify-between gap-2">
							<div class="meta flex flex-col">
								<span class={statusColor[item.status]}>{item.status}</span>
								<span class="truncate normal-case">by {item.uploadedBy}</span>
							</div>
							<div class="flex items-center gap-1">
								<button
									class="btn-icon delete text-gray-500"
									title="Delete"
									onclick={() => remove(item)}
								>
									<Trash2 size={16} />
								</button>
								{#if item.status === 'ready'}
									<button class="btn-round" title="Watch together" onclick={() => watch(item)}>
										<Play size={14} fill="currentColor" />
									</button>
								{/if}
							</div>
						</div>
					</div>
				</article>
			{:else}
				<p class="col-span-full text-muted">Nothing here yet. Upload a video or add a torrent.</p>
			{/each}
		</div>

		<div class="space-y-6">
			<section class="panel">
				<h2 class="panel-title">Rooms</h2>
				<ul class="space-y-2 text-sm">
					{#each rooms as r (r.id)}
						<li class="flex items-baseline justify-between gap-2">
							<a
								href={resolve('/rooms/[id]', { id: r.id })}
								class="truncate font-semibold hover:text-primary hover:underline"
							>
								{r.name}
							</a>
							<span class="meta shrink-0 normal-case">by {r.ownerName}</span>
						</li>
					{:else}
						<li class="text-xs text-muted">No rooms yet.</li>
					{/each}
				</ul>
			</section>
			<Torrents {torrents} onchange={load} />
		</div>
	</div>
</main>
