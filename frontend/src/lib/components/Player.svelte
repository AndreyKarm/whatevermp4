<script lang="ts">
	import { onMount } from 'svelte';
	import Hls from 'hls.js';
	import {
		Maximize,
		Minimize,
		Pause,
		Play,
		RotateCcw,
		RotateCw,
		Volume2,
		VolumeX
	} from '@lucide/svelte';
	import type { MediaItem } from '#lib/types.ts';
	import type { RoomSocket } from '#lib/room.svelte.ts';

	let { item, room, apiUrl }: { item: MediaItem; room: RoomSocket; apiUrl: string } = $props();

	const BACK = 5;
	const FORWARD = 10;

	let container: HTMLDivElement | undefined = $state();
	let videoEl: HTMLVideoElement | undefined = $state();
	let currentTime = $state(0);
	let duration = $state(0);
	let volume = $state(1);
	let muted = $state(false);
	let fullscreen = $state(false);
	let buffered = $state<{ start: number; end: number }[]>([]);
	// Local "no data to play" state (seeking, waiting for segments).
	let stalled = $state(false);
	// Browsers block play() until the user interacts with the page.
	let needsGesture = $state(false);
	// While the seek bar is being dragged, show the drag position instead of the playhead.
	let dragTime = $state<number | null>(null);
	// Last buffering flag sent, to avoid spamming the server.
	let buffering = false;

	const base = $derived(`${apiUrl}${item.playbackUrl}`);
	const shouldPlay = $derived(!!room.state?.playing && !room.isHolding);
	const waitingFor = $derived(room.members.filter((m) => m.buffering).map((m) => m.name));
	const shownTime = $derived(dragTime ?? currentTime);
	const pct = (t: number) => (duration ? `${(t / duration) * 100}%` : '0%');

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

		const setRate = (r: number) => {
			if (video.playbackRate !== r) video.playbackRate = r;
		};

		// Follow the room clock. Rate changes are kept to a minimum: Chrome's
		// audio time-stretching falls behind the video when playbackRate keeps
		// changing, which showed up as audio lagging after a seek.
		const sync = setInterval(() => {
			const s = room.state;
			if (!s || video.readyState === 0 || video.seeking) return;
			const diff = room.getTargetPosition() - video.currentTime;
			const off = Math.abs(diff);
			if (!shouldPlay) {
				if (off > 0.3) video.currentTime = room.getTargetPosition();
				setRate(s.rate);
			} else if (off > 1) {
				video.currentTime = room.getTargetPosition();
				setRate(s.rate);
			} else if (off > 0.25) {
				// one fixed step to catch up / fall back, held until close enough
				setRate(s.rate * (diff > 0 ? 1.05 : 0.95));
			} else if (off < 0.05) {
				setRate(s.rate);
			}
		}, 500);

		const onFullscreen = () => (fullscreen = document.fullscreenElement === container);
		document.addEventListener('fullscreenchange', onFullscreen);

		return () => {
			clearInterval(sync);
			document.removeEventListener('fullscreenchange', onFullscreen);
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

	function setStalled(v: boolean) {
		stalled = v;
		setBuffering(v);
	}

	function updateBuffered() {
		const b = videoEl?.buffered;
		if (!b) return;
		buffered = Array.from({ length: b.length }, (_, i) => ({ start: b.start(i), end: b.end(i) }));
	}

	function togglePlay() {
		if (!videoEl || !room.canControl) return;
		if (room.state?.playing) room.send({ type: 'pause' });
		else room.send({ type: 'play', position: videoEl.currentTime });
	}

	function seekTo(t: number) {
		if (!room.canControl) return;
		room.send({ type: 'seek', position: Math.min(Math.max(t, 0), duration || t) });
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

	function toggleFullscreen() {
		if (document.fullscreenElement) document.exitFullscreen();
		else container?.requestFullscreen();
	}

	function onKey(e: KeyboardEvent) {
		const t = e.target as HTMLElement;
		if (t.closest('input, textarea, select, [contenteditable]') || e.ctrlKey || e.metaKey) return;
		// a focused button already handles space itself
		if (e.key === ' ' && t.closest('button')) return;
		const handlers: Record<string, () => void> = {
			' ': togglePlay,
			k: togglePlay,
			ArrowLeft: () => seekTo(currentTime - BACK),
			ArrowRight: () => seekTo(currentTime + FORWARD),
			f: toggleFullscreen,
			m: () => (muted = !muted)
		};
		const h = handlers[e.key];
		if (h) {
			e.preventDefault();
			h();
		}
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

<svelte:window onkeydown={onKey} />

<div bind:this={container} class="player" class:paused={!shouldPlay}>
	<video
		bind:this={videoEl}
		bind:currentTime
		bind:duration
		bind:volume
		bind:muted
		poster={item.posterKey ? `${base}/poster.jpg` : undefined}
		onwaiting={() => setStalled(true)}
		onseeking={() => (stalled = true)}
		onseeked={() => videoEl && videoEl.readyState >= 3 && (stalled = false)}
		oncanplay={() => setStalled(false)}
		onplaying={() => setStalled(false)}
		onprogress={updateBuffered}
		ontimeupdate={updateBuffered}
		crossorigin="anonymous"
		playsinline
	>
		{#each item.subtitleTracks as t (t.n)}
			<track kind="subtitles" src="{base}/{t.vtt}" srclang={t.lang} label={t.title || t.lang} />
		{/each}
	</video>

	{#if needsGesture}
		<button class="overlay gesture" onclick={resume}>
			<span class="btn-round big"><Play size={24} fill="currentColor" /></span>
			Click to join playback
		</button>
	{:else if room.isHolding || stalled}
		<div class="overlay waiting">
			<div class="spinner"></div>
			{#if room.isHolding && waitingFor.length}
				<span>Waiting for {waitingFor.join(', ')}…</span>
			{/if}
		</div>
	{/if}

	<div class="controls">
		<div class="timeline">
			<div class="track">
				{#each buffered as b, i (i)}
					<div class="buffered" style:left={pct(b.start)} style:width={pct(b.end - b.start)}></div>
				{/each}
				<div class="played" style:width={pct(shownTime)}></div>
			</div>
			<input
				type="range"
				class="scrub"
				aria-label="Seek"
				min="0"
				max={duration || 0}
				step="0.1"
				value={shownTime}
				disabled={!room.canControl}
				oninput={(e) => (dragTime = Number((e.target as HTMLInputElement).value))}
				onchange={(e) => {
					seekTo(Number((e.target as HTMLInputElement).value));
					dragTime = null;
				}}
			/>
		</div>

		<div class="bar">
			{#if room.canControl}
				<button class="btn-round" title="Play / pause (space)" onclick={togglePlay}>
					{#if room.state?.playing}
						<Pause size={14} fill="currentColor" />
					{:else}
						<Play size={14} fill="currentColor" />
					{/if}
				</button>
				<button
					class="btn-icon skip"
					title="Back {BACK}s (←)"
					onclick={() => seekTo(currentTime - BACK)}
				>
					<RotateCcw size={18} /><span>{BACK}</span>
				</button>
				<button
					class="btn-icon skip"
					title="Forward {FORWARD}s (→)"
					onclick={() => seekTo(currentTime + FORWARD)}
				>
					<RotateCw size={18} /><span>{FORWARD}</span>
				</button>
			{/if}
			<span class="time">{fmt(shownTime)} / {fmt(duration)}</span>
			<span class="spacer"></span>
			{#if room.canControl}
				<select class="rate" aria-label="Speed" value={room.state?.rate ?? 1} onchange={setRate}>
					{#each [0.5, 0.75, 1, 1.25, 1.5, 2] as r (r)}
						<option value={r}>{r}×</option>
					{/each}
				</select>
			{/if}
			<button
				class="btn-icon"
				title={muted ? 'Unmute (m)' : 'Mute (m)'}
				onclick={() => (muted = !muted)}
			>
				{#if muted || volume === 0}<VolumeX size={18} />{:else}<Volume2 size={18} />{/if}
			</button>
			<input
				type="range"
				class="volume"
				aria-label="Volume"
				min="0"
				max="1"
				step="0.05"
				bind:value={volume}
			/>
			<button class="btn-icon" title="Fullscreen (f)" onclick={toggleFullscreen}>
				{#if fullscreen}<Minimize size={18} />{:else}<Maximize size={18} />{/if}
			</button>
		</div>
	</div>
</div>

<style>
	.player {
		position: relative;
		width: 100%;
		max-height: 100%;
		aspect-ratio: 16 / 9;
		overflow: hidden;
		border-radius: var(--radius);
		background-color: black;
	}

	.player:fullscreen {
		border-radius: 0;
	}

	video {
		display: block;
		width: 100%;
		height: 100%;
	}

	.overlay {
		position: absolute;
		inset: 0;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.75rem;
		font-size: 13px;
		font-weight: 700;
		text-transform: uppercase;
	}

	.gesture {
		background-color: rgb(0 0 0 / 0.6);
	}

	.big {
		width: 3.5rem;
		height: 3.5rem;
	}

	.waiting {
		pointer-events: none;
		background-color: rgb(0 0 0 / 0.35);
	}

	.controls {
		position: absolute;
		right: 0;
		bottom: 0;
		left: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		padding: 2.5rem 1rem 0.75rem;
		background: linear-gradient(to top, rgb(0 0 0 / 0.9), transparent);
		opacity: 0;
		transition: opacity 0.2s ease;
	}

	.player:hover .controls,
	.player.paused .controls,
	.controls:focus-within {
		opacity: 1;
	}

	/* seek bar: buffered ranges and progress drawn under a transparent range input */
	.timeline {
		position: relative;
		height: 1rem;
	}

	.track {
		position: absolute;
		top: 50%;
		right: 0;
		left: 0;
		height: 4px;
		overflow: hidden;
		border-radius: 999px;
		background-color: rgb(255 255 255 / 0.2);
		transform: translateY(-50%);
		transition: height 0.15s ease;
	}

	.timeline:hover .track {
		height: 6px;
	}

	.buffered,
	.played {
		position: absolute;
		top: 0;
		bottom: 0;
	}

	.buffered {
		background-color: rgb(255 255 255 / 0.4);
	}

	.played {
		left: 0;
		background-color: var(--color-primary);
	}

	.scrub {
		position: absolute;
		inset: 0;
		width: 100%;
		margin: 0;
		background: transparent;
		appearance: none;
		cursor: pointer;
	}

	.scrub:disabled {
		cursor: default;
	}

	.scrub::-webkit-slider-runnable-track {
		background: transparent;
	}

	.scrub::-moz-range-track {
		background: transparent;
	}

	.scrub::-webkit-slider-thumb {
		width: 14px;
		height: 14px;
		border: none;
		border-radius: 50%;
		background: var(--color-primary);
		appearance: none;
		opacity: 0;
		transition: opacity 0.15s ease;
	}

	.scrub::-moz-range-thumb {
		width: 14px;
		height: 14px;
		border: none;
		border-radius: 50%;
		background: var(--color-primary);
		opacity: 0;
	}

	.timeline:hover .scrub:not(:disabled)::-webkit-slider-thumb {
		opacity: 1;
	}

	.timeline:hover .scrub:not(:disabled)::-moz-range-thumb {
		opacity: 1;
	}

	.bar {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 12px;
		font-weight: 600;
	}

	.skip {
		position: relative;
	}

	.skip span {
		position: absolute;
		font-size: 7px;
		font-weight: 800;
	}

	.time {
		margin-left: 0.25rem;
		font-variant-numeric: tabular-nums;
	}

	.spacer {
		flex: 1;
	}

	.rate {
		padding: 0.15rem 0.3rem;
		border: 1px solid var(--color-border);
		border-radius: 0.4rem;
		background-color: var(--color-bg);
		font-size: 12px;
	}

	.volume {
		width: 5rem;
	}
</style>
