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
	import { ArrowLeft, MessageSquare, Trash2, Users } from '@lucide/svelte';

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

<div class="flex flex-col gap-6 p-6 lg:h-[calc(100dvh-var(--header-height))] lg:flex-row">
	<div class="flex min-w-0 flex-1 flex-col gap-4">
		<div class="flex flex-wrap items-center gap-x-4 gap-y-2">
			<a href={resolve('/')} class="btn-icon text-gray-400" title="Back to library">
				<ArrowLeft size={18} />
			</a>
			<h1 class="min-w-0 truncate text-lg uppercase">{info?.name ?? 'Watch Party'}</h1>
			{#if room}
				<span class="meta normal-case">host: {room.owner || '…'}</span>
				{#if !room.connected && !room.fatal}
					<span class="meta text-warn">reconnecting…</span>
				{/if}
			{/if}
			{#if room?.canControl && library.length}
				<select
					class="input ml-auto max-w-64 py-2"
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
		</div>

		<div class="flex min-h-0 flex-1 items-center justify-center">
			{#if room?.fatal}
				<p class="text-accent">This room is gone ({room.fatal}).</p>
			{:else if room && mediaItem?.playbackUrl}
				{#key mediaItem.id}
					<Player item={mediaItem} {room} apiUrl={PUBLIC_API_URL} />
				{/key}
			{:else if mediaItem}
				<p class="text-muted">"{mediaItem.filename}" is {mediaItem.status}.</p>
			{:else}
				<p class="text-muted">No media selected in room.</p>
			{/if}
		</div>
		{#if room?.error || actionError}
			<p class="rounded-lg border border-accent/40 bg-accent/10 px-4 py-2 text-sm text-accent">
				{room?.error ?? actionError}
			</p>
		{/if}
	</div>

	<aside class="flex min-h-96 w-full flex-col overflow-hidden rounded-2xl bg-secondary lg:w-80">
		<div class="border-b border-border p-5">
			<h2 class="panel-title flex items-center gap-2">
				<Users size={16} /> Watching ({room?.members.length ?? 0})
			</h2>
			<ul class="space-y-1 text-sm">
				{#each room?.members ?? [] as m, i (i)}
					<li class="flex items-center gap-2">
						<span class="truncate">{m.name}</span>
						{#if m.name === room?.owner}<span class="badge">host</span>{/if}
						{#if m.buffering}<span class="meta text-warn">buffering</span>{/if}
					</li>
				{/each}
			</ul>
			{#if isOwner}
				<label class="mt-4 flex items-center gap-2 text-sm">
					<input
						type="checkbox"
						class="h-4.5 w-4.5"
						checked={room?.everyone}
						onchange={toggleEveryone}
					/>
					Everyone can control playback
				</label>
				<button class="btn btn-outline mt-4 w-full" onclick={deleteRoom}>
					<Trash2 size={14} /> Delete room
				</button>
			{/if}
		</div>
		<h2 class="panel-title flex items-center gap-2 border-b border-border px-5 py-4">
			<MessageSquare size={16} /> Chat
		</h2>
		<div class="flex-1 space-y-2 overflow-y-auto px-5 pb-4">
			{#each room?.chat ?? [] as msg, i (i)}
				<div class="text-sm break-words">
					<span class="font-bold text-primary">{msg.from.name}:</span>
					<span>{msg.text}</span>
				</div>
			{/each}
		</div>
		<div class="border-t border-border p-4">
			<input
				type="text"
				placeholder="Message..."
				maxlength="500"
				class="input w-full"
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
