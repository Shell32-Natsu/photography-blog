// A web worker that makes the thumbnail of one photo and uploads it.
//
// The Worker on the server cannot do this: decoding a 26-megapixel original
// needs more memory and CPU time than a request there is given. The browser
// has both, so the page does the work and hands over the result.

const LONGEST_EDGE = 1024;
const QUALITY = 0.85;

self.onmessage = async ({ data: { key } }) => {
	try {
		const query = "?key=" + encodeURIComponent(key);
		const original = await fetch("/api/original" + query);
		if (!original.ok) throw new Error(await reason(original));

		const thumbnail = await makeThumbnail(await original.blob());
		const stored = await fetch("/api/thumbnail" + query, { method: "PUT", body: thumbnail });
		if (!stored.ok) throw new Error(await reason(stored));
		self.postMessage({ key, ok: true });
	} catch (err) {
		self.postMessage({ key, ok: false, error: String(err?.message ?? err) });
	}
};

async function reason(response) {
	const body = await response.json().catch(() => null);
	return body?.error ?? `HTTP ${response.status}`;
}

async function makeThumbnail(blob) {
	// "from-image" applies the rotation recorded in EXIF: the thumbnail carries
	// no EXIF data, so its pixels have to be the right way up.
	const bitmap = await createImageBitmap(blob, { imageOrientation: "from-image" });
	const [width, height] = fit(bitmap.width, bitmap.height);
	const canvas = new OffscreenCanvas(width, height);
	const context = canvas.getContext("2d");
	try {
		const source = pixels(bitmap);
		const data = resample(source.data, bitmap.width, bitmap.height, width, height);
		context.putImageData(new ImageData(data, width, height), 0, 0);
	} catch {
		// Some devices cannot hold the full picture in a canvas; let the
		// browser scale it down instead.
		context.imageSmoothingQuality = "high";
		context.drawImage(bitmap, 0, 0, width, height);
	} finally {
		bitmap.close();
	}
	return canvas.convertToBlob({ type: "image/jpeg", quality: QUALITY });
}

/** The size a picture has when scaled down to fit a LONGEST_EDGE square. Small pictures stay as they are. */
function fit(width, height) {
	if (width <= LONGEST_EDGE && height <= LONGEST_EDGE) return [width, height];
	const short = (long, other) => Math.max(1, Math.floor((LONGEST_EDGE * other) / long));
	return width > height ? [LONGEST_EDGE, short(width, height)] : [short(height, width), LONGEST_EDGE];
}

function pixels(bitmap) {
	const canvas = new OffscreenCanvas(bitmap.width, bitmap.height);
	const context = canvas.getContext("2d", { willReadFrequently: true });
	context.drawImage(bitmap, 0, 0);
	return context.getImageData(0, 0, bitmap.width, bitmap.height);
}

/**
 * Scales RGBA pixels with a Lanczos filter, first across, then down. This is
 * the resampling the thumbnails have always been made with; the browser's own
 * scaling is softer and differs from one browser to the next.
 */
function resample(source, sourceWidth, sourceHeight, width, height) {
	let data = source;

	if (width !== sourceWidth) {
		const { first, weights } = contributions(sourceWidth, width);
		const out = new Uint8ClampedArray(width * sourceHeight * 4);
		for (let y = 0; y < sourceHeight; y++) {
			const row = y * sourceWidth * 4;
			let o = y * width * 4;
			for (let x = 0; x < width; x++) {
				const taps = weights[x];
				let p = row + first[x] * 4;
				let r = 0, g = 0, b = 0, a = 0;
				for (let k = 0; k < taps.length; k++, p += 4) {
					const w = taps[k];
					r += w * data[p];
					g += w * data[p + 1];
					b += w * data[p + 2];
					a += w * data[p + 3];
				}
				out[o++] = round(r);
				out[o++] = round(g);
				out[o++] = round(b);
				out[o++] = round(a);
			}
		}
		data = out;
	}

	if (height !== sourceHeight) {
		const { first, weights } = contributions(sourceHeight, height);
		const stride = width * 4;
		const out = new Uint8ClampedArray(stride * height);
		const sum = new Float64Array(stride);
		for (let y = 0; y < height; y++) {
			const taps = weights[y];
			sum.fill(0);
			for (let k = 0; k < taps.length; k++) {
				const w = taps[k];
				const row = (first[y] + k) * stride;
				for (let i = 0; i < stride; i++) sum[i] += w * data[row + i];
			}
			const o = y * stride;
			for (let i = 0; i < stride; i++) out[o + i] = round(sum[i]);
		}
		data = out;
	}

	return data;
}

/**
 * For every output pixel along one axis: the first source pixel that
 * contributes to it and the weight of each one from there on. When shrinking,
 * the filter is widened by the same factor so that every source pixel counts.
 */
function contributions(sourceSize, size) {
	const ratio = sourceSize / size;
	const scale = Math.max(ratio, 1);
	const radius = Math.ceil(3 * scale);
	const first = new Int32Array(size);
	const weights = new Array(size);
	for (let i = 0; i < size; i++) {
		const center = (i + 0.5) * ratio - 0.5;
		const lo = Math.max(0, Math.ceil(center - radius));
		const hi = Math.min(sourceSize - 1, Math.floor(center + radius));
		const taps = new Float64Array(hi - lo + 1);
		let total = 0;
		for (let k = 0; k < taps.length; k++) total += taps[k] = lanczos((lo + k - center) / scale);
		for (let k = 0; k < taps.length; k++) taps[k] /= total;
		first[i] = lo;
		weights[i] = taps;
	}
	return { first, weights };
}

/** The Lanczos filter with three lobes. */
function lanczos(x) {
	x = Math.abs(x);
	if (x === 0) return 1;
	if (x >= 3) return 0;
	const a = Math.PI * x;
	return (Math.sin(a) / a) * (Math.sin(a / 3) / (a / 3));
}

function round(value) {
	return value <= 0 ? 0 : value >= 255 ? 255 : (value + 0.5) | 0;
}
