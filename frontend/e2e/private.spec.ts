import { expect, test } from '@playwright/test';
import { api, runId, userPage } from './helpers';

const owner = `${runId}-owner`;

test('private rooms are unlisted and need the password or invite link', async ({ browser }) => {
	const name = `${runId} secret`;
	const { id, inviteCode } = await api<{ id: string; inviteCode: string }>(
		owner,
		'POST',
		'/api/rooms',
		{ name, private: true, password: 'popcorn' }
	);
	expect(inviteCode).toBeTruthy();

	// the owner keeps the room open so it isn't reaped
	const ownerPage = await userPage(browser, owner);
	await ownerPage.goto(`/rooms/${id}`);
	await expect(ownerPage.getByRole('heading', { name: 'Watching (1)' })).toBeVisible();
	await expect(ownerPage.getByLabel('Invite link')).toHaveValue(new RegExp(`invite=${inviteCode}`));

	const stranger = await userPage(browser, `${runId}-stranger`);
	await stranger.goto('/');
	await expect(stranger.getByRole('heading', { name: 'Rooms' })).toBeVisible();
	await expect(stranger.getByText(name)).toHaveCount(0);

	await stranger.goto(`/rooms/${id}`);
	await expect(stranger.getByText('This room is private')).toBeVisible();
	await stranger.getByLabel('Room password').fill('butter');
	await stranger.getByRole('button', { name: 'Join' }).click();
	await expect(stranger.getByText('wrong password')).toBeVisible();
	await stranger.getByLabel('Room password').fill('popcorn');
	await stranger.getByRole('button', { name: 'Join' }).click();
	await expect(stranger.getByRole('heading', { name: 'Watching (2)' })).toBeVisible();

	const friend = await userPage(browser, `${runId}-friend`);
	await friend.goto(`/rooms/${id}?invite=${inviteCode}`);
	await expect(friend.getByRole('heading', { name: 'Watching (3)' })).toBeVisible();
	await expect(friend.getByText('This room is private')).toHaveCount(0);

	for (const p of [ownerPage, stranger, friend]) await p.context().close();
	await api(owner, 'DELETE', `/api/rooms/${id}`).catch(() => {});
});
