<script lang="ts">
	import { onMount } from 'svelte';
	import Hls from 'hls.js';
	import { Maximize, Pause, Play, Volume2, VolumeX } from '@lucide/svelte';
	import type { MediaItem } from '#lib/types.ts';
	import type { RoomSocket } from '#lib/room.svelte.ts';

	let { item, room, apiUrl }: { item: MediaItem; room: RoomSocket; apiUrl: string } = $props();

	let container: HTMLDivElement | undefined = $state();
	let videoEl: HTMLVideoElement | undefined = $state();
	let currentTime = $state(0);
	let duration = $state(0);
	let volume = $state(1);
	let muted = $state(false);
	// Browsers block play() until the user interacts with the page.
	let needsGesture = $state(false);
	// Last buffering flag sent, to avoid spamming the server.
	let buffering = false;

	const base = $derived(`${apiUrl}${item.playbackUrl}`);
	const shouldPlay = $derived(!!room.state?.playing && !room.isHolding);

	onMount(() => {
		const video = videoEl!;
		let hls: Hls | null = null;
		const src = `${base}/master.m3u8`;
		if (Hls.isSupported()) {
			hls = new Hls();
			hls.loadSource(src);
			hls.attachMedia(video);
		} else if (video.canPlayType('application/vnd.apple.mpegurl')) {
			video.src = src;
		}

		const sync = setInterval(() => {
			const s = room.state;
			if (!s || video.readyState === 0) return;
			const target = room.getTargetPosition();
			const diff = target - video.currentTime;
			if (!shouldPlay) {
				if (Math.abs(diff) > 0.3) video.currentTime = target;
				video.playbackRate = s.rate;
			} else if (Math.abs(diff) > 1.0) {
				video.currentTime = target;
				video.playbackRate = s.rate;
			} else if (Math.abs(diff) > 0.05) {
				// Nudge the rate by up to ±5% to converge without seeking
				video.playbackRate = s.rate * (1 + Math.min(Math.max(diff * 0.1, -0.05), 0.05));
			} else {
				video.playbackRate = s.rate;
			}
		}, 500);

		return () => {
			clearInterval(sync);
			hls?.destroy();
			setBuffering(false);
		};
	});

	$effect(() => {
		if (!videoEl) return;
		if (shouldPlay) {
			videoEl
				.play()
				.then(() => (needsGesture = false))
				.catch(() => (needsGesture = true));
		} else {
			videoEl.pause();
		}
	});

	function setBuffering(v: boolean) {
		if (buffering === v) return;
		buffering = v;
		room.send({ type: 'buffering', buffering: v });
	}

	function togglePlay() {
		if (!videoEl) return;
		if (room.state?.playing) room.send({ type: 'pause' });
		else room.send({ type: 'play', position: videoEl.currentTime });
	}

	// While the seek bar is being dragged, show the drag position instead of the playhead.
	let dragTime = $state<number | null>(null);

	function seek(e: Event) {
		room.send({ type: 'seek', position: Number((e.target as HTMLInputElement).value) });
		dragTime = null;
	}

	function setRate(e: Event) {
		room.send({ type: 'rate', rate: Number((e.target as HTMLSelectElement).value) });
	}

	function resume() {
		videoEl
			?.play()
			.then(() => (needsGesture = false))
			.catch(() => {});
	}

	function fullscreen() {
		if (document.fullscreenElement) document.exitFullscreen();
		else container?.requestFullscreen();
	}

	function fmt(t: number) {
		if (!Number.isFinite(t)) return '0:00';
		const s = Math.floor(t % 60)
			.toString()
			.padStart(2, '0');
		const m = Math.floor(t / 60) % 60;
		const h = Math.floor(t / 3600);
		return h ? `${h}:${m.toString().padStart(2, '0')}:${s}` : `${m}:${s}`;
	}
</script>

<div
	bind:this={container}
	class="group relative aspect-video max-h-full w-full overflow-hidden rounded-2xl bg-black"
>
	<video
		bind:this={videoEl}
		bind:currentTime
		bind:duration
		bind:volume
		bind:muted
		class="w-full h-full"
		poster={item.posterKey ? `${base}/poster.jpg` : undefined}
		onwaiting={() => setBuffering(true)}
		oncanplay={() => setBuffering(false)}
		onplaying={() => setBuffering(false)}
		crossorigin="anonymous"
		playsinline
	>
		{#each item.subtitleTracks as t (t.n)}
			<track kind="subtitles" src="{base}/{t.vtt}" srclang={t.lang} label={t.title || t.lang} />
		{/each}
	</video>

	{#if needsGesture}
		<button
			class="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-black/60 text-sm font-bold uppercase"
			onclick={resume}
		>
			<span class="btn-round h-14 w-14"><Play size={24} fill="currentColor" /></span>
			Click to join playback
		</button>
	{:else if room.isHolding}
		<div
			class="pointer-events-none absolute inset-0 flex items-center justify-center bg-black/50 text-sm font-bold uppercase"
		>
			Waiting for everyone to buffer…
		</div>
	{/if}

	<div
		class="absolute inset-x-0 bottom-0 flex items-center gap-3 bg-gradient-to-t from-black/90 to-transparent px-4 pt-8 pb-3 text-xs font-semibold opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100"
	>
		{#if room.canControl}
			<button class="btn-round" title={room.state?.playing ? 'Pause' : 'Play'} onclick={togglePlay}>
				{#if room.state?.playing}
					<Pause size={14} fill="currentColor" />
				{:else}
					<Play size={14} fill="currentColor" />
				{/if}
			</button>
		{/if}
		<span class="tabular-nums">{fmt(currentTime)}</span>
		<input
			type="range"
			class="min-w-0 flex-1"
			min="0"
			max={duration || 0}
			step="0.1"
			value={dragTime ?? currentTime}
			disabled={!room.canControl}
			oninput={(e) => (dragTime = Number((e.target as HTMLInputElement).value))}
			onchange={seek}
		/>
		<span class="tabular-nums">{fmt(duration)}</span>
		{#if room.canControl}
			<select
				class="rounded-md border border-border bg-bg px-1 py-0.5"
				value={room.state?.rate ?? 1}
				onchange={setRate}
			>
				{#each [0.5, 0.75, 1, 1.25, 1.5, 2] as r (r)}
					<option value={r}>{r}×</option>
				{/each}
			</select>
		{/if}
		<button class="btn-icon" title={muted ? 'Unmute' : 'Mute'} onclick={() => (muted = !muted)}>
			{#if muted || volume === 0}<VolumeX size={18} />{:else}<Volume2 size={18} />{/if}
		</button>
		<input type="range" class="w-20" min="0" max="1" step="0.05" bind:value={volume} />
		<button class="btn-icon" title="Fullscreen" onclick={fullscreen}>
			<Maximize size={18} />
		</button>
	</div>
</div>
