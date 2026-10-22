<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { PUBLIC_API_URL } from '$app/env/public';
	import { Film, Library, Pause, Play } from '@lucide/svelte';
	import { apiFetch } from '#lib/api.ts';
	import { onChange } from '#lib/events.ts';
	import type { RoomItem } from '#lib/types.ts';

	let rooms = $state<RoomItem[]>([]);
	let loaded = $state(false);
	let error = $state<string | null>(null);

	// Rooms with people in them first, then most recently active.
	const sorted = $derived(
		[...rooms].sort((a, b) => (b.members?.length ?? 0) - (a.members?.length ?? 0))
	);

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

	onMount(() => {
		load();
		return onChange(['rooms', 'media_files'], load);
	});
</script>

<svelte:head><title>Rooms · whatevermp4</title></svelte:head>

<main class="container page">
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
