<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { PUBLIC_API_URL } from '$app/env/public';
	import { Film, Library, Lock, Pause, Play } from '@lucide/svelte';
	import { apiFetch } from '#lib/api.ts';
	import { onChange } from '#lib/events.ts';
	import { listProgress } from '#lib/progress.ts';
	import { formatTime } from '#lib/time.ts';
	import { startRoom } from '#lib/watch.ts';
	import type { MediaItem, RoomItem } from '#lib/types.ts';

	let rooms = $state<RoomItem[]>([]);
	let media = $state<MediaItem[]>([]);
	let loaded = $state(false);
	let error = $state<string | null>(null);

	// Rooms with people in them first, then most recently active.
	const sorted = $derived(
		[...rooms].sort((a, b) => (b.members?.length ?? 0) - (a.members?.length ?? 0))
	);

	// Continue watching: this viewer's saved positions (localStorage) on ready
	// media, most recent first. Re-read whenever the media list reloads.
	const continuing = $derived.by(() => {
		const byId = new Map(media.map((m) => [m.id, m]));
		return listProgress()
			.map((p) => ({ ...p, item: byId.get(p.mediaId) }))
			.filter((p): p is typeof p & { item: MediaItem } => p.item?.status === 'ready')
			.slice(0, 12);
	});

	async function load() {
		try {
			rooms = await apiFetch<RoomItem[]>('/api/rooms');
			error = null;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loaded = true;
		}
	}

	async function loadMedia() {
		try {
			media = await apiFetch<MediaItem[]>('/api/media');
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}

	onMount(() => {
		load();
		loadMedia();
		const offs = [onChange(['rooms', 'media_files'], load), onChange(['media_files'], loadMedia)];
		return () => offs.forEach((off) => off());
	});

	async function resume(item: MediaItem) {
		try {
			await startRoom(item);
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}
</script>

<svelte:head><title>Rooms · whatevermp4</title></svelte:head>

<main class="container page">
	{#if continuing.length}
		<section class="continue">
			<h2 class="panel-title">Continue watching</h2>
			<ul class="row">
				{#each continuing as c (c.mediaId)}
					<li>
						<button
							class="resume"
							title="Resume from {formatTime(c.t)}"
							onclick={() => resume(c.item)}
						>
							<div class="thumb">
								{#if c.item.posterKey && c.item.playbackUrl}
									<img
										src="{PUBLIC_API_URL}{c.item.playbackUrl}/poster.jpg"
										alt=""
										draggable="false"
									/>
								{:else}
									<Film size={28} />
								{/if}
								<span class="play btn-round"><Play size={14} fill="currentColor" /></span>
								{#if c.item.durationSec}
									<div class="bar">
										<div style:width="{Math.min(c.t / c.item.durationSec, 1) * 100}%"></div>
									</div>
								{/if}
							</div>
							<span class="title">{c.item.filename}</span>
							<span class="meta">
								{formatTime(c.t)}{c.item.durationSec ? ` / ${formatTime(c.item.durationSec)}` : ''}
							</span>
						</button>
					</li>
				{/each}
			</ul>
		</section>
	{/if}

	<div class="page-head">
		<div>
			<h1 class="section-title">Rooms</h1>
			<p class="muted">Join a room to watch together, or start one from the library.</p>
		</div>
		<a href={resolve('/library')} class="btn btn-primary"><Library size={16} /> Library</a>
	</div>

	{#if error}<p class="error-text">{error}</p>{/if}

	{#if sorted.length}
		<ul class="rooms">
			{#each sorted as r (r.id)}
				{@const members = r.members ?? []}
				<li>
					<a href={resolve('/rooms/[id]', { id: r.id })} class="room">
						<div class="thumb">
							{#if r.media?.posterUrl}
								<img src="{PUBLIC_API_URL}{r.media.posterUrl}" alt="" draggable="false" />
							{:else}
								<Film size={32} />
							{/if}
							{#if members.length}
								<span class="badge live">● {members.length} watching</span>
							{/if}
							{#if r.private}
								<span class="badge private" title="Private: only you see it here"
									><Lock size={10} /></span
								>
							{/if}
						</div>
						<div class="body">
							<h2>{r.name}</h2>
							<p class="now">
								{#if r.media}
									{#if r.state.playing}<Play size={12} fill="currentColor" />{:else}<Pause
											size={12}
											fill="currentColor"
										/>{/if}
									<span>{r.media.filename}</span>
								{:else}
									<span class="muted">Nothing selected</span>
								{/if}
							</p>
							<ul class="watchers">
								{#each members as m, i (i)}
									<li class:host={m === r.ownerName}>{m}</li>
								{:else}
									<li class="empty">Nobody here right now</li>
								{/each}
							</ul>
							<p class="meta">host: {r.ownerName}</p>
						</div>
					</a>
				</li>
			{/each}
		</ul>
	{:else if loaded && !error}
		<div class="panel empty-state">
			<Film size={40} />
			<p>No rooms yet.</p>
			<p class="muted">Pick a movie in the library and press play to start one.</p>
			<a href={resolve('/library')} class="btn btn-primary">Open library</a>
		</div>
	{/if}
</main>

<style>
	.continue {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		margin-bottom: 2.5rem;
	}

	.continue .row {
		display: grid;
		grid-auto-columns: minmax(12rem, 14rem);
		grid-auto-flow: column;
		gap: 1.25rem;
		overflow-x: auto;
		padding: 0.25rem 0.25rem 0.75rem;
		scrollbar-width: thin;
	}

	.resume {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		width: 100%;
		text-align: left;
	}

	.resume .thumb {
		border-radius: 0.3rem;
		transition: box-shadow 0.3s ease;
	}

	.resume:hover .thumb,
	.resume:focus-visible .thumb {
		box-shadow: var(--ring);
	}

	.play {
		position: absolute;
		opacity: 0;
		transition: opacity 0.2s ease;
	}

	.resume:hover .play,
	.resume:focus-visible .play {
		opacity: 1;
	}

	.bar {
		position: absolute;
		right: 0;
		bottom: 0;
		left: 0;
		height: 4px;
		background-color: rgb(0 0 0 / 0.5);
	}

	.bar > div {
		height: 100%;
		background-color: var(--color-primary);
	}

	.title {
		display: -webkit-box;
		overflow: hidden;
		font-size: 12px;
		font-weight: 700;
		line-height: 1.4;
		text-transform: uppercase;
		overflow-wrap: anywhere;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
	}

	.rooms {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(17rem, 1fr));
		gap: 1.5rem;
	}

	.room {
		display: flex;
		flex-direction: column;
		height: 100%;
		overflow: hidden;
		border-radius: var(--radius);
		background-color: var(--color-secondary);
		transition: box-shadow 0.3s ease;
	}

	.room:hover {
		color: inherit;
		box-shadow: var(--ring);
	}

	.thumb {
		position: relative;
		display: flex;
		align-items: center;
		justify-content: center;
		aspect-ratio: 16 / 9;
		overflow: hidden;
		background-color: #1c1c23;
		color: var(--color-muted);
	}

	.thumb img {
		width: 100%;
		height: 100%;
		object-fit: cover;
		border-radius: 0;
	}

	.live {
		position: absolute;
		top: 0.75rem;
		left: 0.75rem;
	}

	.private {
		position: absolute;
		top: 0.75rem;
		right: 0.75rem;
		display: flex;
		padding: 0.25rem;
	}

	.body {
		display: flex;
		flex: 1;
		flex-direction: column;
		gap: 0.5rem;
		padding: 1rem 1.25rem 1.25rem;
	}

	h2 {
		font-size: 15px;
		text-transform: uppercase;
		letter-spacing: 0.5px;
		overflow-wrap: anywhere;
	}

	.now {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		font-size: 13px;
		color: var(--color-muted);
	}

	.now span {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.watchers {
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
		margin-top: auto;
		padding-top: 0.25rem;
	}

	.watchers li {
		padding: 0.1rem 0.6rem;
		border: 1px solid var(--color-border);
		border-radius: 999px;
		font-size: 12px;
	}

	.watchers li.host {
		border-color: var(--color-primary);
		color: var(--color-primary);
	}

	.watchers li.empty {
		border: none;
		padding: 0;
		color: var(--color-gray);
	}

	.empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.75rem;
		padding: 3rem 1.5rem;
		text-align: center;
		color: var(--color-muted);
	}

	.empty-state p:first-of-type {
		color: var(--color-text);
		font-weight: 700;
		text-transform: uppercase;
	}
</style>
