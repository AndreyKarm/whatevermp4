<script lang="ts">
	import { enhance } from '$app/forms';
	import type { ActionData } from './$types';

	let { form }: { form: ActionData } = $props();
</script>

<div>
	<form method="POST" use:enhance>
		{#if form?.error}
			<p class="error">{form.error}</p>
		{/if}
		<input type="email" name="email" required />
		<input type="password" name="password" minlength="8" required />
		<button type="submit">Login</button>
	</form>
</div>
