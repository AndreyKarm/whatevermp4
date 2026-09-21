<script lang="ts">
	import { enhance } from '$app/forms';
	import { resolve } from '$app/paths';

	let { data } = $props();
</script>

{#if data.user}
	{JSON.stringify(data.user)}
	<form method="POST" action="/logout" use:enhance>
		<button type="submit">Logout</button>
	</form>
{:else}
	<a href={resolve('/login')}>Login</a>
	<a href={resolve('/register')}>Register</a>
{/if}
