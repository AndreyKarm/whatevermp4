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

		<label for="username">Username:</label>
		<input type="text" name="username" minlength="2" maxlength="32" required />

		<label for="email">Email:</label>
		<input type="email" name="email" required />

		<label for="password">Password:</label>
		<input type="password" name="password" minlength="8" maxlength="72" required />

		<button type="submit">Register</button>
	</form>
</div>
