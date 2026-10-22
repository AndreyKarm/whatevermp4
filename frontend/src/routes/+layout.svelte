<script lang="ts">
	import favicon from '#lib/assets/favicon.svg';
	import { getNicknameContext, setNicknameContext } from '#lib/nickname.svelte.ts';
	import Modal from '#lib/components/Modal.svelte';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { UserRound } from '@lucide/svelte';
	import type { LayoutProps } from './$types';
	import './app.css';

	let { children }: LayoutProps = $props();
	setNicknameContext();
	const nickname = getNicknameContext();

	let editing = $state(false);
	// The nickname lives in localStorage, so the server can't know it; wait for mount
	// to avoid flashing the modal on every full page load.
	let mounted = $state(false);
	onMount(() => (mounted = true));
	const showModal = $derived(mounted && (!nickname.hasNickname || editing));

	const path = $derived(page.url.pathname);

	function save(e: SubmitEvent) {
		e.preventDefault();
		const val = String(new FormData(e.currentTarget as HTMLFormElement).get('nickname')).trim();
		if (!val) return;
		nickname.name = val;
		editing = false;
	}
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>whatevermp4</title>
</svelte:head>

<header class="site-header">
	<div class="container inner">
		<a href={resolve('/')} class="logo">whatever<span>mp4</span></a>
		<nav>
			<a href={resolve('/')} class:active={path === '/' || path.startsWith('/rooms')}>Rooms</a>
			<a href={resolve('/library')} class:active={path.startsWith('/library')}>Library</a>
			{#if mounted && nickname.hasNickname}
				<button class="nick" title="Change nickname" onclick={() => (editing = true)}>
					<UserRound size={18} />
					{nickname.name}
				</button>
			{/if}
		</nav>
	</div>
</header>

<Modal
	open={showModal}
	title={editing ? 'Change nickname' : 'Pick a nickname'}
	dismissable={editing}
	width="26rem"
	onclose={() => (editing = false)}
>
	<form class="nick-form" onsubmit={save}>
		<label class="field">
			<span class="label">Nickname</span>
			<input
				name="nickname"
				type="text"
				maxlength="32"
				placeholder="Nickname..."
				value={nickname.name}
				class="input"
			/>
		</label>
		<div class="actions">
			{#if editing}
				<button type="button" class="btn btn-outline" onclick={() => (editing = false)}>
					Cancel
				</button>
			{/if}
			<button class="btn btn-primary">Continue</button>
		</div>
	</form>
</Modal>

{@render children()}

<style>
	.site-header {
		position: sticky;
		top: 0;
		z-index: 20;
		display: flex;
		align-items: center;
		height: var(--header-height);
		border-bottom: 1px solid var(--color-secondary);
		background-color: color-mix(in srgb, var(--color-bg), transparent 20%);
		backdrop-filter: blur(10px);
	}

	.inner {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}

	.logo {
		font-size: 1.25rem;
		font-weight: 800;
		letter-spacing: 1px;
		text-transform: uppercase;
	}

	.logo span {
		color: var(--color-primary);
	}

	.logo:hover {
		color: inherit;
	}

	nav {
		display: flex;
		align-items: center;
		gap: 1.5rem;
		font-size: 14px;
		font-weight: 600;
		text-transform: uppercase;
	}

	nav a.active {
		color: var(--color-primary);
	}

	.nick {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		text-transform: none;
	}

	.nick:hover {
		color: var(--color-primary);
	}

	.nick-form {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
	}
</style>
