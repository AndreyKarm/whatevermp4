<script lang="ts">
	import { goto } from '$app/navigation';
	import { apiFetch } from '#lib/api.ts';
	import { nickname } from '#lib/nickname.svelte.ts';
	import { PUBLIC_API_URL } from '$app/env/public';
	import { RoomSocket } from '#lib/room.svelte.ts';
	import Player from '#lib/components/Player.svelte';
	import { page } from '$app/state';
	import type { MediaItem, RoomItem } from '#lib/types.ts';
	import { resolve } from '$app/paths';

	const roomId = $derived(page.params.id ?? '');

	let room = $state<RoomSocket | null>(null);
	let info = $state<RoomItem | null>(null);
	let mediaItem = $state<MediaItem | null>(null);
	let library = $state<MediaItem[]>([]);
	let actionError = $state<string | null>(null);

	// Connect once a nickname is chosen; reconnect if it changes.
	$effect(() => {
		if (!roomId || !nickname.hasNickname) return;
		const r = new RoomSocket(roomId, nickname.name, PUBLIC_API_URL);
		r.connect();
		room = r;
		return () => r.close();
	});

	$effect(() => {
		if (!roomId) return;
		apiFetch<RoomItem>(`/api/rooms/${roomId}`)
			.then((r) => (info = r))
			.catch((e) => (actionError = e.message));
	});

	const mediaId = $derived(room?.state?.mediaId);
	const isOwner = $derived(!!room && room.owner === nickname.name);

	$effect(() => {
		const id = mediaId;
		if (!id) {
			mediaItem = null;
			return;
		}
		apiFetch<MediaItem>(`/api/media/${id}`)
			.then((m) => (mediaItem = m))
			.catch((e) => {
				mediaItem = null;
				actionError = e.message;
			});
	});

	// Ready media for the "change media" picker
	$effect(() => {
		if (!room?.canControl) return;
		apiFetch<MediaItem[]>('/api/media')
			.then((list) => (library = list.filter((m) => m.status === 'ready')))
			.catch(console.error);
	});

	async function toggleEveryone(e: Event) {
		const everyoneControls = (e.target as HTMLInputElement).checked;
		try {
			await apiFetch(`/api/rooms/${roomId}`, {
				method: 'PATCH',
				body: JSON.stringify({ everyoneControls })
			});
		} catch (err) {
			actionError = err instanceof Error ? err.message : String(err);
		}
	}

	async function deleteRoom() {
		if (!confirm('Delete this room for everyone?')) return;
		try {
			await apiFetch(`/api/rooms/${roomId}`, { method: 'DELETE' });
			goto(resolve('/'));
		} catch (err) {
			actionError = err instanceof Error ? err.message : String(err);
		}
	}
</script>

<svelte:head><title>{info?.name ?? 'Watch Party'}</title></svelte:head>

<div class="flex flex-col lg:flex-row h-screen">
	<div class="flex-1 flex flex-col bg-black min-w-0">
		<header class="flex items-center gap-4 px-4 py-2 bg-gray-900 text-sm">
			<a href={resolve('/')} class="text-blue-400">← Library</a>
			<h1 class="font-bold truncate">{info?.name ?? 'Watch Party'}</h1>
			{#if room}
				<span class="text-gray-400">host: {room.owner || '…'}</span>
				{#if !room.connected && !room.fatal}<span class="text-yellow-400">reconnecting…</span>{/if}
			{/if}
			{#if room?.canControl && library.length}
				<select
					class="ml-auto bg-gray-800 rounded p-1 max-w-64"
					value={mediaId ?? ''}
					onchange={(e) =>
						room?.send({ type: 'change_media', mediaId: (e.target as HTMLSelectElement).value })}
				>
					<option value="" disabled>Choose media…</option>
					{#each library as m (m.id)}
						<option value={m.id}>{m.filename}</option>
					{/each}
				</select>
			{/if}
		</header>

		<div class="flex-1 flex items-center justify-center">
			{#if room?.fatal}
				<p class="text-red-400">This room is gone ({room.fatal}).</p>
			{:else if room && mediaItem?.playbackUrl}
				{#key mediaItem.id}
					<Player item={mediaItem} {room} apiUrl={PUBLIC_API_URL} />
				{/key}
			{:else if mediaItem}
				<p class="text-gray-500">"{mediaItem.filename}" is {mediaItem.status}.</p>
			{:else}
				<p class="text-gray-500">No media selected in room.</p>
			{/if}
		</div>
		{#if room?.error || actionError}
			<p class="px-4 py-2 bg-red-900/60 text-sm">{room?.error ?? actionError}</p>
		{/if}
	</div>

	<aside class="w-full lg:w-80 bg-gray-800 flex flex-col">
		<div class="p-4 border-b border-gray-700">
			<h2 class="font-bold mb-2">Watching ({room?.members.length ?? 0})</h2>
			<ul class="text-sm space-y-1">
				{#each room?.members ?? [] as m, i (i)}
					<li>
						{m.name}
						{#if m.name === room?.owner}<span class="text-xs text-blue-400">host</span>{/if}
						{#if m.buffering}<span class="text-xs text-yellow-400">buffering</span>{/if}
					</li>
				{/each}
			</ul>
			{#if isOwner}
				<label class="flex items-center gap-2 mt-3 text-sm">
					<input type="checkbox" checked={room?.everyone} onchange={toggleEveryone} />
					Everyone can control playback
				</label>
				<button class="mt-2 text-xs text-red-400" onclick={deleteRoom}>Delete room</button>
			{/if}
		</div>
		<div class="p-4 border-b border-gray-700 font-bold">Chat</div>
		<div class="flex-1 overflow-y-auto p-4 space-y-2">
			{#each room?.chat ?? [] as msg, i (i)}
				<div class="text-sm">
					<span class="text-blue-400 font-bold">{msg.from.name}:</span>
					<span>{msg.text}</span>
				</div>
			{/each}
		</div>
		<div class="p-4 border-t border-gray-700">
			<input
				type="text"
				placeholder="Message..."
				maxlength="500"
				class="w-full bg-gray-700 p-2 rounded"
				onkeydown={(e) => {
					if (e.key === 'Enter' && room) {
						const target = e.target as HTMLInputElement;
						if (!target.value.trim()) return;
						room.send({ type: 'chat', text: target.value });
						target.value = '';
					}
				}}
			/>
		</div>
	</aside>
</div>
