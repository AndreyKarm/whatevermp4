import { apiFetch, apiPost } from './api';

const PART_SIZE = 64 * 1024 * 1024;
const CONCURRENCY = 4;

interface InitResponse {
	mediaId: string;
	uploadId: string;
}

/**
 * PUTs one part to its presigned URL and resolves with the ETag. Uses XHR
 * rather than fetch because only XHR reports upload progress.
 */
function putPart(url: string, blob: Blob, onProgress: (loaded: number) => void): Promise<string> {
	return new Promise((resolve, reject) => {
		const xhr = new XMLHttpRequest();
		xhr.open('PUT', url);
		xhr.upload.onprogress = (e) => onProgress(e.loaded);
		xhr.onload = () => {
			if (xhr.status < 200 || xhr.status >= 300) {
				reject(new Error(`Upload failed (${xhr.status})`));
				return;
			}
			const etag = xhr.getResponseHeader('ETag');
			if (!etag) {
				reject(new Error('Missing ETag. Add ExposeHeaders: ETag to the bucket CORS config.'));
				return;
			}
			onProgress(blob.size);
			resolve(etag);
		};
		xhr.onerror = () => reject(new Error('Network error while uploading'));
		xhr.send(blob);
	});
}

class S3Uploader {
	/** 0..1 while an upload is running, null otherwise */
	progress = $state<number | null>(null);
	filename = $state('');
	/** Media row of the running upload, so its library card can show progress. */
	mediaId = $state<string | null>(null);
	size = $state(0);
	error = $state<string | null>(null);
	/** Files waiting for the running upload to finish (e.g. several dropped at once). */
	queue = $state<{ file: File; folderId: string | null }[]>([]);

	get busy() {
		return this.progress !== null;
	}

	/** Uploads the files one after another; returns at once. */
	enqueue(files: Iterable<File>, folderId: string | null = null) {
		for (const file of files) this.queue.push({ file, folderId });
		if (!this.busy) this.#drain();
	}

	async #drain() {
		while (this.queue.length && !this.busy) {
			const next = this.queue.shift()!;
			// a failure is shown (uploader.error); carry on with the rest
			await this.upload(next.file, next.folderId).catch(() => {});
		}
	}

	async upload(file: File, folderId: string | null = null) {
		if (this.busy) throw new Error('An upload is already running');
		this.filename = file.name;
		this.size = file.size;
		this.mediaId = null;
		this.error = null;
		this.progress = 0;
		let mediaId: string | null = null;
		try {
			const partsCount = Math.max(1, Math.ceil(file.size / PART_SIZE));
			const init = await apiPost<InitResponse>('/api/uploads/init', {
				filename: file.name,
				contentType: file.type,
				folderId
			});
			mediaId = this.mediaId = init.mediaId;
			const { uploadId } = init;

			// bytes sent per part, summed for byte-accurate progress
			const sent = new Array<number>(partsCount).fill(0);
			const report = () => {
				this.progress = sent.reduce((a, b) => a + b, 0) / Math.max(file.size, 1);
			};

			const completedParts: { partNumber: number; etag: string }[] = [];
			for (let i = 0; i < partsCount; i += CONCURRENCY) {
				const batch = Array.from(
					{ length: Math.min(CONCURRENCY, partsCount - i) },
					(_, idx) => i + idx
				);
				await Promise.all(
					batch.map(async (idx) => {
						const start = idx * PART_SIZE;
						const blob = file.slice(start, Math.min(file.size, start + PART_SIZE));
						// presigned just in time, so slow uploads never hold an expired URL
						const { url } = await apiPost<{ url: string }>(`/api/uploads/${mediaId}/presign`, {
							uploadId,
							partNumber: idx + 1
						});
						const etag = await putPart(url, blob, (loaded) => {
							sent[idx] = loaded;
							report();
						});
						completedParts.push({ partNumber: idx + 1, etag });
					})
				);
			}

			await apiPost(`/api/uploads/${mediaId}/complete`, {
				uploadId,
				parts: completedParts.sort((a, b) => a.partNumber - b.partNumber)
			});
		} catch (err) {
			this.error = err instanceof Error ? err.message : 'Upload failed';
			// Don't leave a stuck "uploading" entry; deleting it also aborts the
			// S3 upload. Best effort: the worker's janitor catches what this misses.
			if (mediaId) apiFetch(`/api/media/${mediaId}`, { method: 'DELETE' }).catch(() => {});
			throw err;
		} finally {
			this.progress = null;
			this.mediaId = null;
		}
	}
}

/** One shared uploader, so progress survives closing the upload modal. */
export const uploader = new S3Uploader();
