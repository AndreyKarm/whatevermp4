import { apiPost } from './api';

const PART_SIZE = 64 * 1024 * 1024;
const CONCURRENCY = 4;

interface InitResponse {
	mediaId: string;
	uploadId: string;
	urls: string[];
}

class S3Uploader {
	/** 0..1 while an upload is running, null otherwise */
	progress = $state<number | null>(null);
	filename = $state('');
	error = $state<string | null>(null);

	get busy() {
		return this.progress !== null;
	}

	async upload(file: File) {
		if (this.busy) throw new Error('An upload is already running');
		this.filename = file.name;
		this.error = null;
		this.progress = 0;
		try {
			const partsCount = Math.max(1, Math.ceil(file.size / PART_SIZE));
			const { mediaId, uploadId, urls } = await apiPost<InitResponse>('/api/uploads/init', {
				filename: file.name,
				contentType: file.type,
				parts: partsCount
			});

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

						const res = await fetch(urls[idx], { method: 'PUT', body: blob });
						if (!res.ok) throw new Error(`Part ${idx + 1} failed (${res.status})`);
						const etag = res.headers.get('ETag');
						if (!etag) {
							throw new Error('Missing ETag. Add ExposeHeaders: ETag to the bucket CORS config.');
						}
						completedParts.push({ partNumber: idx + 1, etag });
						this.progress = completedParts.length / partsCount;
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
		}
	}
}

/** One shared uploader, so progress survives closing the upload modal. */
export const uploader = new S3Uploader();
