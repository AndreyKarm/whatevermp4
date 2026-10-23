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
	import { saveProgress } from '#lib/progress.ts';
	import { formatTime as fmt } from '#lib/time.ts';

	let { item, room, apiUrl }: { item: MediaItem; room: RoomSocket; apiUrl: string } = $props();

	const BACK = 5;
	const FORWARD = 10;
	// Preferred quality as a height ("720") or "auto", per viewer.
	const QUALITY_KEY = 'quality';

	let hls: Hls | null = null;
	// Renditions hls.js found, best first; selecting one only affects this viewer.
	let levels = $state<{ index: number; height: number }[]>([]);
	let quality = $state(-1); // level index, -1 = auto
	let playingHeight = $state(0); // what auto picked
	// Only save the position once this viewer has really played something,
	// so the initial 0:00 never overwrites a saved spot.
	let hasPlayed = false;

	let container: HTMLDivElement | undefined = $state();
	let videoEl: HTMLVideoElement | undefined = $state();
	let currentTime = $state(0);
	let duration = $state(0);
	// Volume and mute are per viewer and survive switching movies / reloads.
	const SOUND_KEY = 'sound';
	const sound = readSound();
	let volume = $state(sound.volume);
	let muted = $state(sound.muted);
	let fullscreen = $state(false);
	let buffered = $state<{ start: number; end: number }[]>([]);
	// Local "no data to play" state (seeking, waiting for segments).
	let stalled = $state(false);
	// Browsers block play() until the user interacts with the page.
	let needsGesture = $state(false);
	// Set when hls.js gives up; shown instead of the controls' overlays.
	let loadError = $state<string | null>(null);
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
		const src = `${base}/master.m3u8`;
		if (Hls.isSupported()) {
			const h = new Hls();
			hls = h;
			h.on(Hls.Events.MANIFEST_PARSED, () => {
				levels = h.levels
					.map((l, index) => ({ index, height: l.height }))
					.sort((a, b) => b.height - a.height);
				const preferred = readQuality();
				const match = levels.find((l) => l.height === preferred);
				if (match) {
					h.currentLevel = match.index;
					quality = match.index;
				}
			});
			h.on(Hls.Events.LEVEL_SWITCHED, (_, data) => {
				playingHeight = h.levels[data.level]?.height ?? 0;
			});
			// hls.js stops on fatal errors; network and media errors are usually
			// recoverable (a dropped connection, a decoder hiccup).
			let mediaRecoveries = 0;
			h.on(Hls.Events.ERROR, (_, data) => {
				if (!data.fatal) return;
				if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
					setTimeout(() => h.startLoad(), 2000);
				} else if (data.type === Hls.ErrorTypes.MEDIA_ERROR && mediaRecoveries++ < 3) {
					h.recoverMediaError();
				} else {
					loadError = 'Playback failed. Reload the page to try again.';
				}
			});
			h.loadSource(src);
			h.attachMedia(video);
		} else if (video.canPlayType('application/vnd.apple.mpegurl')) {
			// Safari's native HLS picks the quality itself; no picker then.
			video.src = src;
		}

		const save = () => {
			if (hasPlayed) saveProgress(item.id, video.currentTime, video.duration || 0);
		};
		const saver = setInterval(() => {
			if (!video.paused) save();
		}, 5000);
		video.addEventListener('pause', save);
		window.addEventListener('pagehide', save);

		const applyRate = (r: number) => {
			if (video.playbackRate !== r) video.playbackRate = r;
		};

		// Follow the room clock. Rate changes are kept to a minimum: Chrome's
		// audio time-stretching falls behind the video when playbackRate keeps
		// changing, which showed up as audio lagging after a seek.
		const sync = setInterval(() => {
			const s = room.state;
			if (!s || video.readyState === 0 || video.seeking) return;
			// The room clock keeps running past the end; don't chase it there.
			const end = Number.isFinite(video.duration) ? video.duration : Infinity;
			if (video.ended && room.getTargetPosition() >= end) return;
			const target = Math.min(room.getTargetPosition(), end);
			const diff = target - video.currentTime;
			const off = Math.abs(diff);
			if (!shouldPlay) {
				if (off > 0.3) video.currentTime = target;
				applyRate(s.rate);
			} else if (off > 1) {
				video.currentTime = target;
				applyRate(s.rate);
			} else if (off > 0.25) {
				// one fixed step to catch up / fall back, held until close enough
				applyRate(s.rate * (diff > 0 ? 1.05 : 0.95));
			} else if (off < 0.05) {
				applyRate(s.rate);
			}
		}, 500);

		// Report this viewer's sync health (drift from the room clock and how
		// much is buffered) so controllers can see who's behind.
		const status = setInterval(() => {
			if (!room.state || video.readyState === 0) return;
			const end = Number.isFinite(video.duration) ? video.duration : Infinity;
			const drift = video.currentTime - Math.min(room.getTargetPosition(), end);
			room.send({ type: 'status', drift, ahead: bufferedAhead(video) });
		}, 2000);

		const onFullscreen = () => (fullscreen = document.fullscreenElement === container);
		document.addEventListener('fullscreenchange', onFullscreen);

		return () => {
			save();
			clearInterval(saver);
			clearInterval(sync);
			clearInterval(status);
			video.removeEventListener('pause', save);
			window.removeEventListener('pagehide', save);
			document.removeEventListener('fullscreenchange', onFullscreen);
			hls?.destroy();
			hls = null;
			setBuffering(false);
		};
	});

	function readSound(): { volume: number; muted: boolean } {
		try {
			const s = JSON.parse(localStorage.getItem(SOUND_KEY) ?? '{}');
			return {
				volume: typeof s.volume === 'number' ? Math.min(Math.max(s.volume, 0), 1) : 1,
				muted: s.muted === true
			};
		} catch {
			return { volume: 1, muted: false };
		}
	}

	$effect(() => {
		const s = { volume, muted };
		try {
			localStorage.setItem(SOUND_KEY, JSON.stringify(s));
		} catch {
			/* storage unavailable: the choice just isn't remembered */
		}
	});

	function readQuality(): number | null {
		try {
			const v = Number(localStorage.getItem(QUALITY_KEY));
			return v > 0 ? v : null;
		} catch {
			return null;
		}
	}

	function setQuality(e: Event) {
		const index = Number((e.target as HTMLSelectElement).value);
		quality = index;
		if (hls) hls.currentLevel = index; // -1 switches back to automatic
		const height = levels.find((l) => l.index === index)?.height;
		try {
			localStorage.setItem(QUALITY_KEY, height ? String(height) : 'auto');
		} catch {
			/* storage unavailable: the choice just isn't remembered */
		}
	}

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

	/** Seconds buffered past the playhead. */
	function bufferedAhead(v: HTMLVideoElement) {
		const t = v.currentTime;
		for (let i = 0; i < v.buffered.length; i++) {
			if (v.buffered.start(i) <= t + 0.1 && t <= v.buffered.end(i)) return v.buffered.end(i) - t;
		}
		return 0;
	}

	// Seek bar hover: the time under the pointer and its preview tile.
	let hover = $state<{ x: number; width: number; t: number } | null>(null);

	function onTimelineMove(e: PointerEvent) {
		const el = e.currentTarget as HTMLElement;
		const rect = el.getBoundingClientRect();
		if (!duration || rect.width === 0) return;
		const x = Math.min(Math.max(e.clientX - rect.left, 0), rect.width);
		hover = { x, width: rect.width, t: (x / rect.width) * duration };
	}

	const preview = $derived.by(() => {
		const th = item.thumbnails;
		if (!hover || !th || th.count === 0) return null;
		const index = Math.min(Math.floor(hover.t / th.interval), th.count - 1);
		const perSheet = th.cols * th.rows;
		const sheet = Math.floor(index / perSheet) + 1;
		const cell = index % perSheet;
		return {
			url: `${base}/thumbs/sprite_${String(sheet).padStart(3, '0')}.jpg`,
			pos: `-${(cell % th.cols) * th.width}px -${Math.floor(cell / th.cols) * th.height}px`,
			width: th.width,
			height: th.height
		};
	});

	// Keep the tooltip inside the player.
	const hoverLeft = $derived.by(() => {
		if (!hover) return 0;
		const half = (preview?.width ?? 48) / 2;
		return Math.min(Math.max(hover.x, half), Math.max(hover.width - half, half));
	});

	function togglePlay() {
		if (!videoEl || !room.canControl) return;
		if (room.state?.playing) room.send({ type: 'pause' });
		else room.send({ type: 'play', position: videoEl.currentTime });
	}

	// Last seek we asked for: quick repeated skips build on it, because the
	// playhead hasn't moved yet while the previous seek is in flight.
	let lastSeek = { t: 0, at: 0 };

	function seekTo(t: number) {
		if (!room.canControl) return;
		const target = Math.min(Math.max(t, 0), duration || t);
		lastSeek = { t: target, at: Date.now() };
		room.send({ type: 'seek', position: target });
	}

	function skip(delta: number) {
		const base = Date.now() - lastSeek.at < 1500 ? lastSeek.t : currentTime;
		seekTo(base + delta);
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
			ArrowLeft: () => skip(-BACK),
			ArrowRight: () => skip(FORWARD),
			f: toggleFullscreen,
			m: () => (muted = !muted)
		};
		const h = handlers[e.key];
		if (h) {
			e.preventDefault();
			h();
		}
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
		onplaying={() => {
			hasPlayed = true;
			setStalled(false);
		}}
		onended={() => setStalled(false)}
		onprogress={updateBuffered}
		ontimeupdate={updateBuffered}
		crossorigin="anonymous"
		playsinline
	>
		{#each item.subtitleTracks as t (t.n)}
			<track kind="subtitles" src="{base}/{t.vtt}" srclang={t.lang} label={t.title || t.lang} />
		{/each}
	</video>

	{#if loadError}
		<div class="overlay error">{loadError}</div>
	{:else if needsGesture}
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
		<div
			class="timeline"
			role="presentation"
			onpointermove={onTimelineMove}
			onpointerleave={() => (hover = null)}
		>
			{#if hover}
				<div class="hover" style:left="{hoverLeft}px">
					{#if preview}
						<div
							class="thumb"
							style:width="{preview.width}px"
							style:height="{preview.height}px"
							style:background-image="url('{preview.url}')"
							style:background-position={preview.pos}
						></div>
					{/if}
					<span>{fmt(hover.t)}</span>
				</div>
			{/if}
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
				<button class="btn-icon skip" title="Back {BACK}s (←)" onclick={() => skip(-BACK)}>
					<RotateCcw size={18} /><span>{BACK}</span>
				</button>
				<button class="btn-icon skip" title="Forward {FORWARD}s (→)" onclick={() => skip(FORWARD)}>
					<RotateCw size={18} /><span>{FORWARD}</span>
				</button>
			{/if}
			<span class="time">{fmt(shownTime)} / {fmt(duration)}</span>
			<span class="spacer"></span>
			{#if levels.length > 1}
				<select
					class="rate"
					aria-label="Quality"
					title="Quality (only for you)"
					value={quality}
					onchange={setQuality}
				>
					<option value={-1}>Auto{playingHeight ? ` (${playingHeight}p)` : ''}</option>
					{#each levels as l (l.index)}
						<option value={l.index}>{l.height}p</option>
					{/each}
				</select>
			{/if}
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

	.error {
		background-color: rgb(0 0 0 / 0.75);
		color: var(--color-accent);
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

	/* disabled inputs swallow pointer events; let the timeline see them for the preview */
	.scrub:disabled {
		cursor: default;
		pointer-events: none;
	}

	.hover {
		position: absolute;
		bottom: 1.25rem;
		z-index: 1;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.25rem;
		transform: translateX(-50%);
		pointer-events: none;
	}

	.hover .thumb {
		border: 1px solid rgb(255 255 255 / 0.6);
		border-radius: 0.25rem;
		background-color: black;
		background-repeat: no-repeat;
	}

	.hover span {
		padding: 0.1rem 0.4rem;
		border-radius: 0.25rem;
		background-color: rgb(0 0 0 / 0.8);
		font-size: 11px;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
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
