import { expect, test } from '@playwright/test';
import { api, EP1, EP2, runId, uploadViaApi, userPage, waitReady } from './helpers';

const hostNick = `${runId}-host`;
const guestNick = `${runId}-guest`;
let folderId = '';
let ep1 = '';
let ep2 = '';

test.describe.configure({ mode: 'serial' });

test.beforeAll(async () => {
	({ id: folderId } = await api<{ id: string }>(hostNick, 'POST', '/api/folders', {
		name: `${runId} Room show`
	}));
	ep1 = await uploadViaApi(hostNick, EP1, folderId);
	ep2 = await uploadViaApi(hostNick, EP2, folderId);
	await waitReady(ep1);
	await waitReady(ep2);
});

test.afterAll(async () => {
	for (const id of [ep1, ep2]) await api(hostNick, 'DELETE', `/api/media/${id}`).catch(() => {});
	if (folderId) await api(hostNick, 'DELETE', `/api/folders/${folderId}`).catch(() => {});
});

test('two viewers: sync, chat, sync status, queue and handing over control', async ({
	browser
}) => {
	const { id } = await api<{ id: string }>(hostNick, 'POST', '/api/rooms', {
		name: `${runId} party`,
		mediaId: ep1
	});
	const host = await userPage(browser, hostNick);
	const guest = await userPage(browser, guestNick);
	await host.goto(`/rooms/${id}`);
	await guest.goto(`/rooms/${id}`);
	for (const p of [host, guest]) {
		await expect(p.getByRole('heading', { name: 'Watching (2)' })).toBeVisible();
		await expect(p.locator('.player video')).toBeVisible();
	}

	// only the host has playback controls; pressing play starts everyone
	await expect(guest.getByTitle('Play / pause (space)')).toHaveCount(0);
	await host.getByTitle('Play / pause (space)').click();
	await expect(guest.locator('.player')).not.toHaveClass(/paused/);
	await expect
		.poll(() => guest.locator('video').evaluate((v: HTMLVideoElement) => v.currentTime))
		.toBeGreaterThan(0.5);

	// chat reaches the other viewer
	await guest.getByPlaceholder('Message…').fill('hello from the couch');
	await guest.getByPlaceholder('Message…').press('Enter');
	await expect(host.getByText('hello from the couch')).toBeVisible();

	// everyone's ping is listed; controllers also get a sync-health dot
	const guestRow = host.locator('.members li', { hasText: guestNick });
	await expect(guestRow.locator('.ping')).toHaveText(/^\d+ ms$/, { timeout: 20_000 });
	await expect(guestRow.locator('.dot')).toHaveAttribute('title', /buffered/);

	// queue the next episode (season/episode order), then skip to it
	await host.getByRole('button', { name: 'Next episodes' }).click();
	await expect(host.getByRole('heading', { name: 'Up next (1)' })).toBeVisible();
	await expect(guest.getByRole('heading', { name: 'Up next (1)' })).toBeVisible();
	await expect(guest.locator('.queue')).toContainText('Test.Show.S01E02.mp4');
	await host.getByRole('button', { name: 'Play next' }).click();
	await expect(host.getByLabel('Change movie')).toHaveValue(ep2);
	await expect(guest.getByText('Nothing queued')).toHaveCount(0); // controls hidden for guests
	await expect(host.getByText('Nothing queued')).toBeVisible();

	// give the guest control, then the whole room
	await guestRow.getByTitle('Let them control playback').click();
	await expect(guest.getByTitle('Play / pause (space)')).toBeVisible();
	await expect(guestRow.getByText('controls')).toBeVisible();
	host.once('dialog', (d) => d.accept());
	await guestRow.getByTitle('Make host').click();
	await expect(guest.getByText(`host: ${guestNick}`)).toBeVisible();
	await expect(guest.getByRole('button', { name: 'Delete room' })).toBeVisible();
	await expect(host.getByRole('button', { name: 'Delete room' })).toHaveCount(0);

	await host.context().close();
	await guest.context().close();
});
