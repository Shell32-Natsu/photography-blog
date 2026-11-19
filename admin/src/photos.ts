import { HttpError, type Env } from "./index";

export interface Photo {
	key: string;
	size: number;
	uploaded: string;
	tags: string[];
	/** Whether the thumbnail bucket has this photo. */
	thumb: boolean;
}

/** A photo's tags are kept on the object itself, in this metadata field, separated by commas. */
const TAG_FIELD = "tag";

const MAX_TAGS = 32;
const MAX_TAG_LENGTH = 64;
const MAX_THUMBNAIL_BYTES = 8 << 20;

export async function listPhotos(env: Env): Promise<Photo[]> {
	const [originals, thumbs] = await Promise.all([listAll(env.PHOTOS, true), listAll(env.THUMBS, false)]);
	const withThumb = new Set(thumbs.map((object) => object.key));
	return originals.map((object) => ({
		key: object.key,
		size: object.size,
		uploaded: object.uploaded.toISOString(),
		tags: parseTags(object.customMetadata?.[TAG_FIELD]),
		thumb: withThumb.has(object.key),
	}));
}

/** Lists a whole bucket, in key order. */
async function listAll(bucket: R2Bucket, withMetadata: boolean): Promise<R2Object[]> {
	const objects: R2Object[] = [];
	let cursor: string | undefined;
	do {
		// A page can hold fewer objects than asked for; `truncated` is what
		// says whether there are more.
		const page = await bucket.list({ cursor, limit: 1000, include: withMetadata ? ["customMetadata"] : undefined });
		objects.push(...page.objects);
		cursor = page.truncated ? page.cursor : undefined;
	} while (cursor);
	return objects;
}

export function parseTags(value: string | undefined): string[] {
	const tags = (value ?? "").split(",").map((tag) => tag.trim());
	return [...new Set(tags.filter(Boolean))];
}

/** Checks tags sent by the page and puts them in their stored form. */
function cleanTags(input: unknown): string[] {
	if (!Array.isArray(input) || input.some((tag) => typeof tag !== "string")) {
		throw new HttpError(400, "标签必须是字符串列表。");
	}
	const tags = [...new Set((input as string[]).map((tag) => tag.trim()).filter(Boolean))];
	if (tags.length > MAX_TAGS) throw new HttpError(400, `一张照片最多 ${MAX_TAGS} 个标签。`);
	for (const tag of tags) {
		// A tag becomes an album title and a path on the site.
		if (tag.length > MAX_TAG_LENGTH || /[,\p{Cc}]/u.test(tag)) {
			throw new HttpError(400, `「${tag}」不能用作标签：不能含逗号，最长 ${MAX_TAG_LENGTH} 个字符。`);
		}
	}
	return tags;
}

/**
 * Replaces a photo's tags. R2 cannot change metadata in place, so the object
 * is written again under the same key: the body is streamed straight from
 * the read to the write and never held in memory.
 */
export async function setTags(env: Env, key: string, input: unknown): Promise<string[]> {
	const tags = cleanTags(input);
	const object = await env.PHOTOS.get(key);
	if (!object) throw new HttpError(404, `没有名为「${key}」的照片。`);

	const customMetadata = { ...object.customMetadata };
	if (tags.length) customMetadata[TAG_FIELD] = tags.join(",");
	else delete customMetadata[TAG_FIELD];

	const { readable, writable } = new FixedLengthStream(object.size);
	const stop = new AbortController();
	const copying = object.body.pipeTo(writable, { signal: stop.signal }).catch(() => {});
	let written: R2Object | null;
	try {
		written = await env.PHOTOS.put(key, readable, {
			httpMetadata: object.httpMetadata,
			customMetadata,
			storageClass: object.storageClass,
			// Refuse to overwrite a photo that was replaced while we were at it.
			onlyIf: { etagMatches: object.etag },
		});
	} finally {
		// A write that was refused never reads the body; do not leave it waiting.
		stop.abort();
	}
	await copying;
	if (!written) throw new HttpError(409, `「${key}」在此期间被改动过，请刷新后重试。`);
	return tags;
}

/** Streams an original to the page, which needs the pixels to make a thumbnail. */
export async function original(env: Env, key: string): Promise<Response> {
	const object = await env.PHOTOS.get(key);
	if (!object) throw new HttpError(404, `没有名为「${key}」的照片。`);
	const headers = new Headers({ "Content-Length": String(object.size), ETag: object.httpEtag });
	object.writeHttpMetadata(headers);
	return new Response(object.body, { headers });
}

/** Stores a thumbnail that the page made in the browser. */
export async function putThumbnail(env: Env, key: string, request: Request): Promise<{ key: string; size: number }> {
	if (Number(request.headers.get("Content-Length")) > MAX_THUMBNAIL_BYTES) {
		throw new HttpError(413, "这个文件太大，不像是缩略图。");
	}
	const body = new Uint8Array(await request.arrayBuffer());
	if (body.length > MAX_THUMBNAIL_BYTES) throw new HttpError(413, "这个文件太大，不像是缩略图。");
	if (body[0] !== 0xff || body[1] !== 0xd8 || body[2] !== 0xff) {
		throw new HttpError(415, "缩略图必须是 JPEG 文件。");
	}
	if (!(await env.PHOTOS.head(key))) throw new HttpError(404, `没有名为「${key}」的照片。`);

	await env.THUMBS.put(key, body, { httpMetadata: { contentType: "image/jpeg" } });
	return { key, size: body.length };
}
