<script lang="ts">
	import { goto } from '$app/navigation';
	import { apiFetch, apiPost } from '#lib/api.ts';
	import { onChange } from '#lib/events.ts';
	import { MediaList } from '#lib/media.svelte.ts';
	import { getProgress } from '#lib/progress.ts';
	import { nickname } from '#lib/nickname.svelte.ts';
	import { PUBLIC_API_URL } from '$app/env/public';
	import { RoomSocket, saveInvite, savedInvite } from '#lib/room.svelte.ts';
	import Player from '#lib/components/Player.svelte';
	import { page } from '$app/state';
	import type { Folder, LockedRoom, MediaItem, RoomItem, RoomMember } from '#lib/types.ts';
	import { pathLabel } from '#lib/folders.ts';
	import { byFilename, nextEpisodes } from '#lib/episodes.ts';
	import { resolve } from '$app/paths';
	import { browser } from '$app/env';
	import {
		ArrowDown,
		ArrowLeft,
		ArrowUp,
		Copy,
		Crown,
		Gamepad2,
		ListPlus,
		ListVideo,
		Lock,
		MessageSquare,
		PanelRightClose,
		PanelRightOpen,
		Send,
		SkipForward,
		Trash2,
		Users,
		X
	} from '@lucide/svelte';

	const roomId = $derived(page.params.id ?? '');

	let room = $state<RoomSocket | null>(null);
	// Settings come over the room socket (sent on join and on every change);
	// the REST call is only for the name and password prompt of a room that
	// refused us.
	const info = $derived(room?.info ?? null);
	let lockedInfo = $state<LockedRoom | null>(null);
	const mediaList = new MediaList();
	let folders = $state<Folder[]>([]);
	let actionError = $state<string | null>(null);
	let message = $state('');
	let chatEl: HTMLDivElement | undefined = $state();

	// Private rooms: the invite code from the link (?invite=…) or remembered
	// from an earlier visit or a password; the socket reconnects when it changes.
	let invite = $state<string | null>(null);
	let password = $state('');
	let unlocking = $state(false);
	let newPassword = $state('');
	let copied = $state(false);

	$effect.pre(() => {
		if (!browser || !roomId) return;
		const fromLink = page.url.searchParams.get('invite');
		if (fromLink) saveInvite(roomId, fromLink);
		invite = fromLink ?? savedInvite(roomId);
	});

	// Side panel (members, settings, chat) can be hidden to give the player
	// the full width; remembered per viewer.
	const PANEL_KEY = 'room-panel-collapsed';
	let collapsed = $state(readCollapsed());
	// Chat messages seen while the panel was open, for the unread badge.
	let seenChat = $state(0);
	const unread = $derived(collapsed ? Math.max((room?.chat.length ?? 0) - seenChat, 0) : 0);

	function readCollapsed() {
		if (!browser) return false;
		try {
			return localStorage.getItem(PANEL_KEY) === '1';
		} catch {
			return false;
		}
	}

	function togglePanel() {
		collapsed = !collapsed;
		try {
			localStorage.setItem(PANEL_KEY, collapsed ? '1' : '0');
		} catch {
			/* storage unavailable: the choice just isn't remembered */
		}
	}

	$effect(() => {
		if (!collapsed) seenChat = room?.chat.length ?? 0;
	});

	// Connect once a nickname is chosen; reconnect if it or the invite changes.
	$effect(() => {
		if (!roomId || !nickname.hasNickname) return;
		const r = new RoomSocket(roomId, nickname.name, PUBLIC_API_URL, invite);
		r.connect();
		room = r;
		return () => r.close();
	});

	// Refused (private room, no valid invite): fetch the name and whether a
	// password works. The owner gets the invite code instead and reconnects.
	$effect(() => {
		if (!room?.locked) return;
		apiFetch<RoomItem | LockedRoom>(`/api/rooms/${roomId}`)
			.then((r) => {
				if ('locked' in r) {
					lockedInfo = r;
				} else if (r.inviteCode) {
					saveInvite(roomId, r.inviteCode);
					invite = r.inviteCode;
				}
			})
			.catch((e) => (actionError = e.message));
	});

	// The owner gets the invite code with the settings; remember it for reloads.
	$effect(() => {
		if (info?.inviteCode) saveInvite(roomId, info.inviteCode);
	});

	const locked = $derived(!!room?.locked);
	const joined = $derived(!!info);
	const roomName = $derived(info?.name ?? lockedInfo?.name ?? 'Watch Party');
	const mediaId = $derived(room?.state?.mediaId);
	const isOwner = $derived(!!room && room.owner === nickname.name);
	// The selected media and the picker's ready media, kept current from server events.
	const mediaItem = $derived(mediaList.items.find((m) => m.id === mediaId) ?? null);
	const library = $derived(mediaList.items.filter((m) => m.status === 'ready'));
	const byId = $derived(new Map(library.map((m) => [m.id, m])));
	const inviteLink = $derived(
		info?.inviteCode && browser
			? `${page.url.origin}${resolve('/rooms/[id]', { id: roomId })}?invite=${info.inviteCode}`
			: ''
	);

	// One row per nickname (the same name can be connected twice).
	const members = $derived<RoomMember[]>([
		...new Map((room?.members ?? []).map((m) => [m.name, m])).values()
	]);

	function loadFolders() {
		apiFetch<Folder[]>('/api/folders')
			.then((list) => (folders = list))
			.catch(console.error);
	}

	// Picker options grouped by folder path ("Show / Season 1"), top level first.
	const pickerGroups = $derived.by(() => {
		const groups: Record<string, MediaItem[]> = {};
		for (const m of library) {
			const label = m.folderId ? pathLabel(folders, m.folderId) || 'Library' : 'Library';
			(groups[label] ??= []).push(m);
		}
		return Object.entries(groups)
			.map(([label, items]) => ({ label, items: items.sort(byFilename) }))
			.sort((a, b) =>
				a.label === 'Library' ? -1 : b.label === 'Library' ? 1 : a.label.localeCompare(b.label)
			);
	});

	// Load the library once we're in; events keep it current after that.
	$effect(() => {
		if (!joined) return;
		loadFolders();
		const offs = [mediaList.live(), onChange(['folders'], loadFolders)];
		return () => offs.forEach((off) => off());
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

	function errorText(err: unknown) {
		return err instanceof Error ? err.message : String(err);
	}

	async function patchRoom(body: Record<string, unknown>) {
		try {
			// the change comes back over the room socket
			await apiFetch(`/api/rooms/${roomId}`, { method: 'PATCH', body: JSON.stringify(body) });
		} catch (err) {
			actionError = errorText(err);
		}
	}

	function toggleController(name: string) {
		const list = room?.controllers ?? [];
		patchRoom({
			controllers: list.includes(name) ? list.filter((n) => n !== name) : [...list, name]
		});
	}

	function makeHost(name: string) {
		if (!confirm(`Make ${name} the host? You'll lose the host settings.`)) return;
		patchRoom({ ownerName: name });
	}

	function setPassword(e: SubmitEvent) {
		e.preventDefault();
		patchRoom({ password: newPassword });
		newPassword = '';
	}

	async function copyInvite() {
		try {
			await navigator.clipboard.writeText(inviteLink);
			copied = true;
			setTimeout(() => (copied = false), 2000);
		} catch {
			actionError = 'Copy failed; select the link and copy it yourself.';
		}
	}

	async function unlock(e: SubmitEvent) {
		e.preventDefault();
		unlocking = true;
		try {
			const { inviteCode } = await apiPost<{ inviteCode: string }>(`/api/rooms/${roomId}/access`, {
				password
			});
			saveInvite(roomId, inviteCode);
			invite = inviteCode;
			password = '';
			actionError = null;
		} catch (err) {
			actionError = errorText(err);
		} finally {
			unlocking = false;
		}
	}

	function setQueue(queue: string[]) {
		room?.send({ type: 'queue', queue });
	}

	function moveInQueue(i: number, delta: number) {
		const q = [...(room?.queue ?? [])];
		const j = i + delta;
		if (j < 0 || j >= q.length) return;
		[q[i], q[j]] = [q[j], q[i]];
		setQueue(q);
	}

	function queueNextEpisodes() {
		if (!mediaItem) return;
		const queued = new Set(room?.queue ?? []);
		const add = nextEpisodes(library, mediaItem)
			.map((m) => m.id)
			.filter((id) => !queued.has(id));
		if (!add.length) {
			actionError = 'No later episodes in this folder.';
			return;
		}
		setQueue([...(room?.queue ?? []), ...add]);
	}

	async function deleteRoom() {
		if (!confirm('Delete this room for everyone?')) return;
		try {
			await apiFetch(`/api/rooms/${roomId}`, { method: 'DELETE' });
			goto(resolve('/'));
		} catch (err) {
			actionError = errorText(err);
		}
	}

	// Sync health of a member, from its last status report.
	function syncClass(m: RoomMember) {
		if (m.drift == null) return '';
		const off = Math.abs(m.drift);
		return off < 0.3 ? 'sync-good' : off < 1 ? 'sync-warn' : 'sync-bad';
	}

	function pingClass(ms: number) {
		return ms < 150 ? '' : ms < 400 ? 'ping-warn' : 'ping-bad';
	}

	function fmtDrift(d: number) {
		return `${d >= 0 ? '+' : '−'}${Math.abs(d).toFixed(1)}s`;
	}
</script>

<svelte:head><title>{roomName} · whatevermp4</title></svelte:head>

<div class="room-page">
	<section class="stage">
		<div class="head">
			<a href={resolve('/')} class="btn-icon back" title="Back to rooms"><ArrowLeft size={18} /></a>
			<h1>{roomName}</h1>
			{#if info?.private}<span class="meta" title="Private room"><Lock size={12} /></span>{/if}
			{#if room && !locked}
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
					onchange={(e) => {
						const id = (e.target as HTMLSelectElement).value;
						// start where whoever switched last watched it
						room?.send({ type: 'change_media', mediaId: id, position: getProgress(id) ?? 0 });
					}}
				>
					<option value="" disabled>Choose a movie…</option>
					{#each pickerGroups as g (g.label)}
						<optgroup label={g.label}>
							{#each g.items as m (m.id)}
								<option value={m.id}>{m.filename}</option>
							{/each}
						</optgroup>
					{/each}
				</select>
			{/if}
			<button
				class="btn-icon panel-toggle"
				title={collapsed ? 'Show room panel' : 'Hide room panel'}
				aria-expanded={!collapsed}
				onclick={togglePanel}
			>
				{#if collapsed}<PanelRightOpen size={18} />{:else}<PanelRightClose size={18} />{/if}
				{#if unread}<span class="badge unread">{unread}</span>{/if}
			</button>
		</div>

		<div class="screen">
			{#if room?.fatal}
				<p class="error-text">This room is gone ({room.fatal}).</p>
			{:else if locked}
				<div class="panel lock">
					<Lock size={32} />
					<p class="lock-title">This room is private</p>
					{#if lockedInfo?.hasPassword}
						<form class="lock-form" onsubmit={unlock}>
							<input
								class="input"
								type="password"
								placeholder="Room password"
								aria-label="Room password"
								autocomplete="off"
								bind:value={password}
							/>
							<button class="btn btn-primary" disabled={!password || unlocking}>Join</button>
						</form>
					{:else}
						<p class="muted">Ask the host for an invite link.</p>
					{/if}
				</div>
			{:else if room && mediaItem?.playbackUrl}
				<!-- remount per movie; a re-process unmounts it anyway (status → processing) -->
				{#key mediaItem.id}
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

	<aside class="side" hidden={collapsed || locked}>
		<div class="block">
			<h2 class="panel-title"><Users size={16} /> Watching ({members.length})</h2>
			<ul class="members">
				{#each members as m (m.name)}
					{@const controls = room?.controllers.includes(m.name)}
					<li>
						{#if room?.canControl && m.drift != null}
							<span
								class="dot {syncClass(m)}"
								title="{fmtDrift(m.drift)} from the room, {Math.round(m.ahead ?? 0)}s buffered"
							></span>
						{/if}
						<span class="name">{m.name}</span>
						{#if m.name === room?.owner}
							<span class="badge">host</span>
						{:else if controls}
							<span class="badge ctl">controls</span>
						{/if}
						{#if m.buffering}<span class="meta status-processing">buffering</span>{/if}
						{#if m.ping != null}
							<span class="meta ping {pingClass(m.ping)}" title="Round trip to the server">
								{m.ping} ms
							</span>
						{/if}
						{#if isOwner && m.name !== room?.owner}
							<span class="member-actions">
								<button
									class="btn-icon"
									title={controls ? 'Take away control' : 'Let them control playback'}
									aria-pressed={controls}
									onclick={() => toggleController(m.name)}
								>
									<Gamepad2 size={14} />
								</button>
								<button class="btn-icon" title="Make host" onclick={() => makeHost(m.name)}>
									<Crown size={14} />
								</button>
							</span>
						{/if}
					</li>
				{/each}
			</ul>
			{#if isOwner && info}
				<label class="toggle">
					<input
						type="checkbox"
						checked={room?.everyone}
						onchange={(e) =>
							patchRoom({ everyoneControls: (e.target as HTMLInputElement).checked })}
					/>
					Everyone can control playback
				</label>
				<label class="toggle">
					<input
						type="checkbox"
						checked={info.private}
						onchange={(e) => patchRoom({ private: (e.target as HTMLInputElement).checked })}
					/>
					Private (not listed, invite only)
				</label>
				{#if info.private}
					<div class="invite">
						<input class="input" readonly value={inviteLink} aria-label="Invite link" />
						<button class="btn-icon" title="Copy invite link" onclick={copyInvite}>
							<Copy size={14} />
						</button>
					</div>
					<div class="invite-actions">
						<span class="meta">{copied ? 'Copied' : ''}</span>
						<button
							class="meta link"
							title="Old links stop working"
							onclick={() => patchRoom({ resetInvite: true })}
						>
							New link
						</button>
					</div>
					<form class="invite" onsubmit={setPassword}>
						<input
							class="input"
							type="password"
							autocomplete="new-password"
							placeholder={info.hasPassword ? 'New password' : 'Password (optional)'}
							aria-label="Room password"
							bind:value={newPassword}
						/>
						<button class="btn btn-outline" disabled={!newPassword}>Set</button>
					</form>
					{#if info.hasPassword}
						<button class="meta link" onclick={() => patchRoom({ password: '' })}>
							Remove password
						</button>
					{/if}
				{/if}
				<button class="btn btn-danger" onclick={deleteRoom}>
					<Trash2 size={14} /> Delete room
				</button>
			{/if}
		</div>

		{#if room && (room.canControl || room.queue.length)}
			<div class="block">
				<h2 class="panel-title"><ListVideo size={16} /> Up next ({room.queue.length})</h2>
				{#if room.queue.length}
					<ol class="queue">
						{#each room.queue as id, i (id)}
							<li>
								<span class="name" title={byId.get(id)?.filename}>
									{byId.get(id)?.filename ?? '…'}
								</span>
								{#if room.canControl}
									<button
										class="btn-icon"
										title="Move up"
										disabled={i === 0}
										onclick={() => moveInQueue(i, -1)}
									>
										<ArrowUp size={14} />
									</button>
									<button
										class="btn-icon"
										title="Move down"
										disabled={i === room.queue.length - 1}
										onclick={() => moveInQueue(i, 1)}
									>
										<ArrowDown size={14} />
									</button>
									<button
										class="btn-icon"
										title="Remove"
										onclick={() => setQueue(room!.queue.filter((q) => q !== id))}
									>
										<X size={14} />
									</button>
								{/if}
							</li>
						{/each}
					</ol>
				{:else}
					<p class="muted">Nothing queued. The room stops when this ends.</p>
				{/if}
				{#if room.canControl}
					<div class="queue-actions">
						{#if room.queue.length}
							<button class="btn btn-outline" onclick={() => room?.send({ type: 'next' })}>
								<SkipForward size={14} /> Play next
							</button>
						{/if}
						{#if mediaItem}
							<button
								class="btn btn-outline"
								title="Add the later episodes of this folder, in season/episode order"
								onclick={queueNextEpisodes}
							>
								<ListPlus size={14} /> Next episodes
							</button>
						{/if}
					</div>
					{#if library.length}
						<select
							class="input"
							aria-label="Add to queue"
							value=""
							onchange={(e) => {
								const el = e.target as HTMLSelectElement;
								if (el.value && !room?.queue.includes(el.value))
									setQueue([...(room?.queue ?? []), el.value]);
								el.value = '';
							}}
						>
							<option value="" disabled>Add to queue…</option>
							{#each pickerGroups as g (g.label)}
								<optgroup label={g.label}>
									{#each g.items as m (m.id)}
										<option value={m.id}>{m.filename}</option>
									{/each}
								</optgroup>
							{/each}
						</select>
					{/if}
				{/if}
			</div>
		{/if}

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

	.panel-toggle {
		position: relative;
		margin-left: auto;
		color: var(--color-gray);
	}

	.picker + .panel-toggle {
		margin-left: 0;
	}

	.unread {
		position: absolute;
		top: -0.4rem;
		right: -0.4rem;
		padding: 0 0.35rem;
	}

	.screen {
		display: flex;
		flex: 1;
		align-items: center;
		justify-content: center;
		min-height: 0;
	}

	.lock {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.75rem;
		width: min(100%, 24rem);
		text-align: center;
		color: var(--color-muted);
	}

	.lock-title {
		color: var(--color-text);
		font-weight: 700;
		text-transform: uppercase;
	}

	.lock-form {
		display: flex;
		gap: 0.5rem;
		width: 100%;
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
		overflow: hidden auto;
		border-radius: var(--radius);
		background-color: var(--color-secondary);
	}

	.side[hidden] {
		display: none;
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

	.members,
	.queue {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		margin: 0;
		padding: 0;
		font-size: 14px;
		list-style: none;
	}

	.members li,
	.queue li {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.25rem 0.5rem;
	}

	.name {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.queue .name {
		flex: 1;
	}

	.queue .btn-icon,
	.member-actions .btn-icon {
		min-width: 26px;
		height: 26px;
		color: var(--color-gray);
	}

	.member-actions {
		display: flex;
		margin-left: auto;
	}

	.member-actions [aria-pressed='true'] {
		color: var(--color-primary);
	}

	.ctl {
		border: 1px solid var(--color-primary);
		background: none;
		color: var(--color-primary);
	}

	/* sync health: drift from the room clock */
	.dot {
		flex-shrink: 0;
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background-color: var(--color-gray);
	}

	.dot.sync-good {
		background-color: var(--color-ok);
	}

	.dot.sync-warn {
		background-color: var(--color-warn);
	}

	.dot.sync-bad {
		background-color: var(--color-accent);
	}

	.ping {
		font-variant-numeric: tabular-nums;
		text-transform: none;
	}

	.ping-warn {
		color: var(--color-warn);
	}

	.ping-bad {
		color: var(--color-accent);
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

	.invite {
		display: flex;
		gap: 0.5rem;
	}

	.invite .input {
		padding: 0.5rem 0.75rem;
		font-size: 12px;
	}

	.invite-actions {
		display: flex;
		justify-content: space-between;
	}

	.link:hover {
		color: var(--color-primary);
	}

	.queue-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}

	.queue-actions .btn {
		padding: 0.4rem 0.75rem;
		font-size: 11px;
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
		min-height: 8rem;
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
