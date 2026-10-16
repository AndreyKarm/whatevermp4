import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { createAuthClient } from 'better-auth/svelte';

export const authClient = createAuthClient();

export const signIn = async () => {
	await authClient.signIn.social({
		provider: 'discord'
	});
};

export const signOut = async () => {
	await authClient.signOut();
	await goto(resolve('/'));
};
