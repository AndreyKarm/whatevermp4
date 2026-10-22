import type { Folder } from './types';

/** Folders from the top level down to `id` (empty for the top level). */
export function folderPath(folders: Folder[], id: string | null): Folder[] {
	const byId = new Map(folders.map((f) => [f.id, f]));
	const path: Folder[] = [];
	const seen = new Set<string>();
	for (let f = id ? byId.get(id) : undefined; f && !seen.has(f.id);) {
		seen.add(f.id);
		path.unshift(f);
		f = f.parentId ? byId.get(f.parentId) : undefined;
	}
	return path;
}

/** "Show / Season 1" style label. */
export function pathLabel(folders: Folder[], id: string): string {
	return folderPath(folders, id)
		.map((f) => f.name)
		.join(' / ');
}

/** `id` and every folder below it. */
export function subtree(folders: Folder[], id: string): Set<string> {
	const out = new Set([id]);
	let grew = true;
	while (grew) {
		grew = false;
		for (const f of folders) {
			if (f.parentId && out.has(f.parentId) && !out.has(f.id)) {
				out.add(f.id);
				grew = true;
			}
		}
	}
	return out;
}

/** All folders as {id, label} options sorted by path, optionally leaving some out. */
export function folderOptions(folders: Folder[], exclude = new Set<string>()) {
	return folders
		.filter((f) => !exclude.has(f.id))
		.map((f) => ({ id: f.id, label: pathLabel(folders, f.id) }))
		.sort((a, b) => a.label.localeCompare(b.label, undefined, { numeric: true }));
}

export const byNumericName = (a: { name: string }, b: { name: string }) =>
	a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' });
