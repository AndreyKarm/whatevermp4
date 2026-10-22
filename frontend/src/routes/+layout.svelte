<script lang="ts">
	import favicon from '#lib/assets/favicon.svg';
	import { getNicknameContext, setNicknameContext } from '#lib/nickname.svelte.ts';
	import { onMount } from 'svelte';
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

<header
	class="sticky top-0 z-20 flex h-(--header-height) items-center border-b border-secondary bg-bg/80 backdrop-blur"
>
	<div class="mx-auto flex w-full max-w-[75em] items-center justify-between px-8">
		<a href={resolve('/')} class="text-xl font-extrabold tracking-wide uppercase">
			whatever<span class="text-primary">mp4</span>
		</a>
		<nav class="flex items-center gap-6 text-sm font-semibold uppercase">
			<a href={resolve('/')} class="hover:text-primary">Library</a>
			{#if nickname.hasNickname}
				<button
					class="flex items-center gap-2 normal-case hover:text-primary"
					title="Change nickname"
					onclick={() => (editing = true)}
				>
					<UserRound size={18} />
					{nickname.name}
				</button>
			{/if}
		</nav>
	</div>
</header>

{#if showModal}
	<div class="modal-overlay">
		<form class="modal-content flex flex-col gap-4" onsubmit={save}>
			<h2 class="text-xl uppercase">{editing ? 'Change nickname' : 'Pick a nickname'}</h2>
			<label class="flex flex-col gap-2">
				<span class="label">Nickname</span>
				<!-- svelte-ignore a11y_autofocus -->
				<input
					name="nickname"
					type="text"
					maxlength="32"
					placeholder="Nickname..."
					value={nickname.name}
					class="input"
					autofocus
				/>
			</label>
			<div class="flex justify-end gap-3">
				{#if editing}
					<button type="button" class="btn btn-outline" onclick={() => (editing = false)}>
						Cancel
					</button>
				{/if}
				<button class="btn btn-primary">Continue</button>
			</div>
		</form>
	</div>
{/if}

{@render children()}
