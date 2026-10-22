<script lang="ts">
	import { goto } from '$app/navigation';
	import { apiFetch } from '#lib/api.ts';
	import { onChange } from '#lib/events.ts';
	import { nickname } from '#lib/nickname.svelte.ts';
	import { PUBLIC_API_URL } from '$app/env/public';
	import { RoomSocket } from '#lib/room.svelte.ts';
	import Player from '#lib/components/Player.svelte';
	import { page } from '$app/state';
	import type { MediaItem, RoomItem } from '#lib/types.ts';
	import { resolve } from '$app/paths';
	import { ArrowLeft, MessageSquare, Send, Trash2, Users } from '@lucide/svelte';

	const roomId = $derived(page.params.id ?? '');

	let room = $state<RoomSocket | null>(null);
	let info = $state<RoomItem | null>(null);
	let mediaItem = $state<MediaItem | null>(null);
	let library = $state<MediaItem[]>([]);
	let actionError = $state<string | null>(null);
	let message = $state('');
	let chatEl: HTMLDivElement | undefined = $state();

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

	function loadMedia(id: string) {
		apiFetch<MediaItem>(`/api/media/${id}`)
			.then((m) => (mediaItem = m))
			.catch((e) => {
				mediaItem = null;
				actionError = e.message;
			});
	}

	function loadLibrary() {
		apiFetch<MediaItem[]>('/api/media')
			.then((list) => (library = list.filter((m) => m.status === 'ready')))
			.catch(console.error);
	}

	// The selected media, kept fresh (e.g. after a re-process) via server events.
	$effect(() => {
		const id = mediaId;
		if (!id) {
			mediaItem = null;
			return;
		}
		loadMedia(id);
		return onChange(['media_files'], () => loadMedia(id));
	});

	// Ready media for the "change media" picker
	$effect(() => {
		if (!room?.canControl) return;
		loadLibrary();
		return onChange(['media_files'], loadLibrary);
	});

	// Keep the newest chat message in view.
	$effect(() => {
		if (room?.chat.length && chatEl) chatEl.scrollTop = chatEl.scrollHeight;
	});

	function sendChat(e: SubmitEvent) {
		e.preventDefault();
		if (!room || !message.trim()) return;
		room.send({ type: 'chat', text: message });
		message = '';
	}

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

<svelte:head><title>{info?.name ?? 'Watch Party'} · whatevermp4</title></svelte:head>

<div class="room-page">
	<section class="stage">
		<div class="head">
			<a href={resolve('/')} class="btn-icon back" title="Back to rooms"><ArrowLeft size={18} /></a>
			<h1>{info?.name ?? 'Watch Party'}</h1>
			{#if room}
				<span class="meta">host: {room.owner || '…'}</span>
				{#if !room.connected && !room.fatal}
					<span class="meta reconnecting">reconnecting…</span>
				{/if}
			{/if}
			{#if room?.canControl && library.length}
				<select
					class="input picker"
					aria-label="Change movie"
					value={mediaId ?? ''}
					onchange={(e) =>
						room?.send({ type: 'change_media', mediaId: (e.target as HTMLSelectElement).value })}
				>
					<option value="" disabled>Choose a movie…</option>
					{#each library as m (m.id)}
						<option value={m.id}>{m.filename}</option>
					{/each}
				</select>
			{/if}
		</div>

		<div class="screen">
			{#if room?.fatal}
				<p class="error-text">This room is gone ({room.fatal}).</p>
			{:else if room && mediaItem?.playbackUrl}
				{#key `${mediaItem.id}:${mediaItem.playbackUrl}`}
					<Player item={mediaItem} {room} apiUrl={PUBLIC_API_URL} />
				{/key}
			{:else if mediaItem}
				<p class="muted">"{mediaItem.filename}" is {mediaItem.status}.</p>
			{:else}
				<p class="muted">No movie selected yet.</p>
			{/if}
		</div>

		{#if room?.error || actionError}
			<p class="alert">{room?.error ?? actionError}</p>
		{/if}
	</section>

	<aside class="side">
		<div class="block">
			<h2 class="panel-title"><Users size={16} /> Watching ({room?.members.length ?? 0})</h2>
			<ul class="members">
				{#each room?.members ?? [] as m, i (i)}
					<li>
						<span class="name">{m.name}</span>
						{#if m.name === room?.owner}<span class="badge">host</span>{/if}
						{#if m.buffering}<span class="meta status-processing">buffering</span>{/if}
					</li>
				{/each}
			</ul>
			{#if isOwner}
				<label class="toggle">
					<input type="checkbox" checked={room?.everyone} onchange={toggleEveryone} />
					Everyone can control playback
				</label>
				<button class="btn btn-danger" onclick={deleteRoom}>
					<Trash2 size={14} /> Delete room
				</button>
			{/if}
		</div>

		<h2 class="panel-title chat-title"><MessageSquare size={16} /> Chat</h2>
		<div class="chat" bind:this={chatEl}>
			{#each room?.chat ?? [] as msg, i (i)}
				<p><b>{msg.from.name}:</b> {msg.text}</p>
			{:else}
				<p class="muted">No messages yet.</p>
			{/each}
		</div>
		<form class="send" onsubmit={sendChat}>
			<input
				type="text"
				placeholder="Message…"
				maxlength="500"
				class="input"
				bind:value={message}
			/>
			<button class="btn-icon" title="Send" disabled={!message.trim()}><Send size={16} /></button>
		</form>
	</aside>
</div>

<style>
	.room-page {
		display: flex;
		flex-direction: column;
		gap: 1.5rem;
		padding: 1.5rem;
	}

	@media (min-width: 1024px) {
		.room-page {
			flex-direction: row;
			height: calc(100dvh - var(--header-height));
		}
	}

	.stage {
		display: flex;
		flex: 1;
		flex-direction: column;
		gap: 1rem;
		min-width: 0;
	}

	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem 1rem;
	}

	.back {
		color: var(--color-gray);
	}

	h1 {
		min-width: 0;
		overflow: hidden;
		font-size: 1.125rem;
		text-overflow: ellipsis;
		text-transform: uppercase;
		white-space: nowrap;
	}

	.reconnecting {
		color: var(--color-warn);
	}

	.picker {
		width: auto;
		max-width: 16rem;
		margin-left: auto;
		padding: 0.5rem 0.75rem;
	}

	.screen {
		display: flex;
		flex: 1;
		align-items: center;
		justify-content: center;
		min-height: 0;
	}

	.alert {
		padding: 0.5rem 1rem;
		border: 1px solid color-mix(in srgb, var(--color-accent), transparent 60%);
		border-radius: var(--radius-sm);
		background-color: color-mix(in srgb, var(--color-accent), transparent 90%);
		color: var(--color-accent);
		font-size: 14px;
	}

	.side {
		display: flex;
		flex-direction: column;
		min-height: 24rem;
		overflow: hidden;
		border-radius: var(--radius);
		background-color: var(--color-secondary);
	}

	@media (min-width: 1024px) {
		.side {
			width: 20rem;
		}
	}

	.block {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		padding: 1.25rem;
		border-bottom: 1px solid var(--color-border);
	}

	.members {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		font-size: 14px;
	}

	.members li {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.name {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.toggle {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 14px;
	}

	.toggle input {
		width: 18px;
		height: 18px;
	}

	.chat-title {
		padding: 1rem 1.25rem;
		border-bottom: 1px solid var(--color-border);
	}

	.chat {
		display: flex;
		flex: 1;
		flex-direction: column;
		gap: 0.5rem;
		overflow-y: auto;
		padding: 1rem 1.25rem;
		font-size: 14px;
		overflow-wrap: anywhere;
	}

	.chat b {
		color: var(--color-primary);
	}

	.send {
		display: flex;
		gap: 0.5rem;
		padding: 1rem;
		border-top: 1px solid var(--color-border);
	}
</style>
