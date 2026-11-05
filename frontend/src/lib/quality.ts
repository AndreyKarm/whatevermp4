// The usual names; a frame within 7% of one gets it (1904x800, 1920x1088).
const TIERS = [4320, 2160, 1440, 1080, 720, 480, 360, 240, 144];

/**
 * The "720p" number of a frame: the height of the 16:9 frame it matches,
 * whatever its shape. 1280x546 (21:9) and 720x1280 (portrait) are both 720p.
 * Same rule as `qualityHeight` in backend/internal/transcode/hls.go.
 */
export function qualityHeight(width: number, height: number): number {
	if (!width || !height) return height;
	const long = Math.max(width, height);
	const q = Math.max(Math.min(width, height), Math.floor((long * 9) / 16));
	return TIERS.find((t) => q * 100 >= t * 93 && q * 100 <= t * 107) ?? q;
}
