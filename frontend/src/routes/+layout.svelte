<script lang="ts">
	import favicon from '#lib/assets/favicon.svg';
	import { getNicknameContext, setNicknameContext } from '#lib/nickname.svelte.ts';
	import type { LayoutProps } from './$types';
	import './app.css';

	let { children }: LayoutProps = $props();
	setNicknameContext();
	const nickname = getNicknameContext();
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
</svelte:head>

<div class="min-h-screen bg-gray-900 text-gray-100">
	{#if !nickname.hasNickname}
		<!-- Nickname Modal rendered directly to avoid "declared but never read" -->
		<div class="fixed inset-0 bg-black/80 flex items-center justify-center z-50">
			<div class="bg-gray-800 p-6 rounded-lg shadow-xl w-80">
				<h2 class="text-xl mb-4 font-bold">Enter Nickname</h2>
				<input
					type="text"
					maxlength="32"
					placeholder="Nickname..."
					class="bg-gray-700 p-2 rounded w-full mb-4 outline-none focus:ring-2 focus:ring-blue-500"
					onkeydown={(e) => {
						if (e.key === 'Enter') {
							const val = (e.target as HTMLInputElement).value.trim();
							if (val) nickname.name = val;
						}
					}}
				/>
				<p class="text-xs text-gray-400">Press Enter to continue</p>
			</div>
		</div>
	{/if}

	{@render children()}
</div>
