import { apiPost } from './api';

const PART_SIZE = 64 * 1024 * 1024;
const CONCURRENCY = 4;

interface InitResponse {
	mediaId: string;
	uploadId: string;
	urls: string[];
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

	get busy() {
		return this.progress !== null;
	}

	async upload(file: File, folderId: string | null = null) {
		if (this.busy) throw new Error('An upload is already running');
		this.filename = file.name;
		this.size = file.size;
		this.mediaId = null;
		this.error = null;
		this.progress = 0;
		try {
			const partsCount = Math.max(1, Math.ceil(file.size / PART_SIZE));
			const { mediaId, uploadId, urls } = await apiPost<InitResponse>('/api/uploads/init', {
				filename: file.name,
				contentType: file.type,
				parts: partsCount,
				folderId
			});
			this.mediaId = mediaId;

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
						const etag = await putPart(urls[idx], blob, (loaded) => {
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
			throw err;
		} finally {
			this.progress = null;
			this.mediaId = null;
		}
	}
}

/** One shared uploader, so progress survives closing the upload modal. */
export const uploader = new S3Uploader();
