<script lang="ts">
	import { HardDriveUpload, Link, Magnet } from '@lucide/svelte';
	import Modal from './Modal.svelte';
	import Uploader from './Uploader.svelte';
	import Torrents from './Torrents.svelte';
	import UrlImport from './UrlImport.svelte';

	let {
		open,
		onclose,
		folderId = null,
		folderLabel = ''
	}: {
		open: boolean;
		onclose: () => void;
		/** folder new movies go into; null = top level */
		folderId?: string | null;
		folderLabel?: string;
	} = $props();

	type Tab = 'device' | 'torrent' | 'link';
	let tab = $state<Tab>('device');

	const tabs = [
		{ id: 'device', label: 'From device', icon: HardDriveUpload },
		{ id: 'torrent', label: 'Torrent', icon: Magnet },
		{ id: 'link', label: '.m3u8 link', icon: Link }
	] as const;
</script>

<Modal {open} title="Add a movie" width="34rem" {onclose}>
	{#if folderLabel}
		<p class="muted">Adding to <b>{folderLabel}</b></p>
	{/if}
	<div class="tabs" role="tablist">
		{#each tabs as t (t.id)}
			<button
				role="tab"
				aria-selected={tab === t.id}
				class:active={tab === t.id}
				onclick={() => (tab = t.id)}
			>
				<t.icon size={16} />
				{t.label}
			</button>
		{/each}
	</div>

	<div role="tabpanel">
		{#if tab === 'device'}
			<Uploader {folderId} onstarted={onclose} />
		{:else if tab === 'torrent'}
			<Torrents {folderId} onadded={onclose} />
		{:else}
			<UrlImport {folderId} onadded={onclose} />
		{/if}
	</div>
</Modal>

<style>
	.tabs {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 0.25rem;
		padding: 0.25rem;
		border-radius: var(--radius-sm);
		background-color: var(--color-bg);
	}

	.tabs button {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.4rem;
		padding: 0.55rem 0.5rem;
		border-radius: 0.4rem;
		font-size: 12px;
		font-weight: 700;
		text-transform: uppercase;
		color: var(--color-gray);
		transition:
			background-color 0.2s ease,
			color 0.2s ease;
	}

	.tabs button:hover {
		color: var(--color-text);
	}

	.tabs button.active {
		background-color: var(--color-primary);
		color: white;
	}
</style>
