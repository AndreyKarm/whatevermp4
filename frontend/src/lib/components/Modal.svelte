<script lang="ts">
	import type { Snippet } from 'svelte';
	import { X } from '@lucide/svelte';

	let {
		open,
		title,
		onclose,
		dismissable = true,
		width = '32rem',
		children
	}: {
		open: boolean;
		title: string;
		onclose?: () => void;
		/** false: no close button, Esc and backdrop clicks are ignored */
		dismissable?: boolean;
		width?: string;
		children: Snippet;
	} = $props();

	let dialog: HTMLDialogElement | undefined = $state();

	// <dialog> gives focus trapping, Esc handling and a backdrop for free.
	$effect(() => {
		if (!dialog) return;
		if (open && !dialog.open) dialog.showModal();
		else if (!open && dialog.open) dialog.close();
	});
</script>

<dialog
	bind:this={dialog}
	style:--modal-width={width}
	oncancel={(e) => {
		e.preventDefault();
		if (dismissable) onclose?.();
	}}
	onclick={(e) => {
		// clicks on the backdrop land on the <dialog> itself
		if (dismissable && e.target === dialog) onclose?.();
	}}
>
	{#if open}
		<div class="body">
			<header>
				<h2>{title}</h2>
				{#if dismissable}
					<button class="btn-icon" aria-label="Close" onclick={onclose}><X size={20} /></button>
				{/if}
			</header>
			{@render children()}
		</div>
	{/if}
</dialog>

<style>
	dialog {
		width: min(var(--modal-width), calc(100vw - 2rem));
		max-height: calc(100dvh - 2rem);
		padding: 0;
		border: none;
		border-radius: var(--radius);
		background: var(--color-secondary);
		color: var(--color-text);
	}

	dialog::backdrop {
		background: rgb(0 0 0 / 0.6);
	}

	.body {
		display: flex;
		flex-direction: column;
		gap: 1.25rem;
		padding: 2rem;
	}

	header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
	}

	h2 {
		font-size: 20px;
		text-transform: uppercase;
	}
</style>
