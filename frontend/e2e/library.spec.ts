import { readFile } from 'node:fs/promises';
import { expect, test } from '@playwright/test';
import { api, EP1, EP2, runId, userPage } from './helpers';

const nick = `${runId}-lib`;
const folderName = `${runId} Show`;

test.describe.configure({ mode: 'serial' });

test.afterAll(async () => {
	const media = await api<{ id: string; uploadedBy: string }[]>(nick, 'GET', '/api/media');
	for (const m of media.filter((m) => m.uploadedBy === nick)) {
		await api(nick, 'DELETE', `/api/media/${m.id}`).catch(() => {});
	}
	const folders = await api<{ id: string; name: string }[]>(nick, 'GET', '/api/folders');
	for (const f of folders.filter((f) => f.name.startsWith(runId))) {
		await api(nick, 'DELETE', `/api/folders/${f.id}`).catch(() => {});
	}
});

test('pick a nickname, upload into a folder, search and bulk-move', async ({ page }) => {
	await page.goto('/library');
	await expect(page.getByRole('heading', { name: 'Pick a nickname' })).toBeVisible();
	await page.locator('input[name=nickname]').fill(nick);
	await page.getByRole('button', { name: 'Continue' }).click();
	await expect(page.getByRole('button', { name: nick })).toBeVisible();

	await page.getByRole('button', { name: 'New folder' }).first().click();
	await page.getByPlaceholder('Show name, Season 1, …').fill(folderName);
	await page.getByRole('button', { name: 'Create' }).click();
	await page.getByRole('link', { name: folderName }).last().click();
	await expect(page).toHaveURL(/folder=/);
	await expect(page.getByRole('heading', { name: folderName })).toBeVisible();

	await page.getByRole('button', { name: 'Add movie' }).first().click();
	await page.locator('input[type=file]').setInputFiles([EP1, EP2]);

	const card = (name: string) => page.locator('li.card', { hasText: name });
	for (const name of ['Test.Show.S01E01.mp4', 'Test.Show.S01E02.mp4']) {
		await expect(card(name).locator('.status-ready')).toBeVisible({ timeout: 150_000 });
	}
	await expect(card('Test.Show.S01E01.mp4').getByText('S1E1')).toBeVisible();

	await page.getByLabel('Search by name').fill('s01e02');
	await expect(page.locator('li.card')).toHaveCount(1);
	await page.getByTitle('Clear filters').click();
	await expect(page.locator('li.card')).toHaveCount(2);

	await page.getByRole('button', { name: 'Select', exact: true }).click();
	await page.getByRole('button', { name: 'Select Test.Show.S01E01.mp4' }).click();
	await page.getByRole('button', { name: 'Select Test.Show.S01E02.mp4' }).click();
	await expect(page.getByText('2 selected')).toBeVisible();
	await page.getByLabel('Move to folder').selectOption({ label: '— Top level —' });
	await expect(page.getByText('This folder is empty.')).toBeVisible();

	// both are at the top level now; "Whole library" finds them from any folder
	await page.getByLabel('Where to search').selectOption('all');
	await page.getByLabel('Search by name').fill('Test.Show.S01E0');
	await expect(page.locator('li.card', { hasText: `by ${nick}` })).toHaveCount(2);
});

test('dropping a file onto the library uploads it into the open folder', async ({ browser }) => {
	const page = await userPage(browser, nick);
	await page.goto('/library');
	await page.getByRole('link', { name: folderName }).last().click();
	await expect(page.getByRole('heading', { name: folderName })).toBeVisible();

	const b64 = (await readFile(EP1)).toString('base64');
	await page.evaluate((b64) => {
		const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
		const dt = new DataTransfer();
		dt.items.add(new File([bytes], 'Dropped.S02E01.mp4', { type: 'video/mp4' }));
		const fire = (type: string) =>
			document.body.dispatchEvent(
				new DragEvent(type, { dataTransfer: dt, bubbles: true, cancelable: true })
			);
		fire('dragenter');
		fire('dragover');
		fire('drop');
	}, b64);

	const card = page.locator('li.card', { hasText: 'Dropped.S02E01.mp4' });
	await expect(card).toBeVisible();
	await expect(card.locator('.status-ready')).toBeVisible({ timeout: 150_000 });
	await expect(card.getByText('S2E1')).toBeVisible();
	await page.context().close();
});

test('edit modal: actions for a ready movie, and cancelling an upload removes it', async ({
	browser
}) => {
	// an upload that never sends its parts stays "uploading"
	await api(nick, 'POST', '/api/uploads/init', {
		filename: 'Stuck.Upload.mp4',
		contentType: 'video/mp4'
	});
	const page = await userPage(browser, nick);
	await page.goto('/library');
	const modal = page.locator('dialog[open]');

	const ready = page.locator('li.card', { hasText: 'Test.Show.S01E01.mp4' });
	await ready.getByTitle('Edit').click();
	const actions = modal.locator('.actions');
	const footer = modal.locator('footer');
	for (const name of ['Watch together', 'Re-process', 'Delete original']) {
		await expect(actions.getByRole('button', { name, exact: true })).toBeVisible();
	}
	await expect(modal.getByRole('button', { name: /^Cancel/ })).toHaveCount(0);
	// Delete sits left in the footer, Close and Save right
	const x = async (name: string) =>
		(await footer.getByRole('button', { name, exact: true }).boundingBox())!.x;
	expect(await x('Delete')).toBeLessThan(await x('Close'));
	expect(await x('Close')).toBeLessThan(await x('Save'));
	await footer.getByRole('button', { name: 'Close' }).click();

	const stuck = page.locator('li.card', { hasText: 'Stuck.Upload.mp4' });
	await stuck.getByTitle('Edit').click();
	await expect(modal.getByRole('button', { name: 'Watch together' })).toHaveCount(0);
	page.once('dialog', (d) => d.accept());
	await modal.getByRole('button', { name: 'Cancel upload' }).click();
	await expect(page.locator('dialog[open]')).toHaveCount(0);
	await expect(stuck).toHaveCount(0);
	await page.context().close();
});

test('continue watching and the storage overview', async ({ browser }) => {
	const media = await api<{ id: string; filename: string; uploadedBy: string }[]>(
		nick,
		'GET',
		'/api/media'
	);
	const ep1 = media.find((m) => m.uploadedBy === nick && m.filename === 'Test.Show.S01E01.mp4')!;
	const page = await userPage(browser, nick);
	await page.goto('/');
	await page.evaluate(
		(id) => localStorage.setItem(`progress:${id}`, JSON.stringify({ t: 5, at: Date.now() })),
		ep1.id
	);
	await page.reload();
	const row = page.locator('section.continue');
	await expect(row.getByRole('heading', { name: 'Continue watching' })).toBeVisible();
	await row.getByRole('button', { name: /Test\.Show\.S01E01/ }).click();
	await expect(page).toHaveURL(/\/rooms\//);
	await expect(page.locator('.player video')).toBeVisible();

	await page.goto('/library/storage');
	await expect(page.getByRole('heading', { name: 'Storage' })).toBeVisible();
	const item = page.locator('.row', { hasText: 'Test.Show.S01E02.mp4' }).last();
	await expect(item).toBeVisible();
	page.once('dialog', (d) => d.accept());
	await item.getByTitle('Delete the original').click();
	await expect(item.getByText('deleted')).toBeVisible();
	await page.context().close();
});
