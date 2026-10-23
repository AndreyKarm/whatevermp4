/** 1536 → "2 KB", 5_000_000 → "4.8 MB". */
export function formatBytes(n: number): string {
	if (n < 1024 ** 2) return `${(n / 1024).toFixed(0)} KB`;
	if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(1)} MB`;
	if (n < 1024 ** 4) return `${(n / 1024 ** 3).toFixed(2)} GB`;
	return `${(n / 1024 ** 4).toFixed(2)} TB`;
}
