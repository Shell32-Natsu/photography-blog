// The admin for the photography blog: tag the photos in R2, make their
// thumbnails, and publish the albums that the tags describe.

import { listPhotos, original, putThumbnail, setTags } from "./photos";
import { draft, publish } from "./publish";

export interface Env {
	ASSETS: Fetcher;
	/** Original photos. */
	PHOTOS: R2Bucket;
	/** Thumbnails, under the same keys as the originals. */
	THUMBS: R2Bucket;
	/** Public addresses of the two buckets. */
	PHOTO_BASE_URL: string;
	THUMB_BASE_URL: string;
	/** Where config.json lives: "owner/name" and the branch to commit to. */
	GITHUB_REPO: string;
	GITHUB_BRANCH: string;
	GITHUB_API_URL?: string;
	GITHUB_RAW_URL?: string;
	/** Secret: the password for everything served here. */
	ADMIN_TOKEN?: string;
	/** Secret: lets "publish" commit config.json. Without it the file can only be downloaded. */
	GITHUB_TOKEN?: string;
}

/** An error whose message is meant for the person using the admin. */
export class HttpError extends Error {
	constructor(
		readonly status: number,
		message: string,
	) {
		super(message);
	}
}

export default {
	async fetch(request, env): Promise<Response> {
		let response: Response;
		try {
			response = (await refuse(request, env)) ?? (await route(request, env));
		} catch (err) {
			if (!(err instanceof HttpError)) console.error(err);
			response =
				err instanceof HttpError
					? json({ error: err.message }, err.status)
					: json({ error: "服务器出错了。" }, 500);
		}
		return harden(response, env);
	},
} satisfies ExportedHandler<Env>;

async function route(request: Request, env: Env): Promise<Response> {
	const url = new URL(request.url);
	const key = () => {
		const key = url.searchParams.get("key");
		if (!key) throw new HttpError(400, "缺少照片的 key。");
		return key;
	};

	switch (`${request.method} ${url.pathname}`) {
		case "GET /api/photos":
			return json({
				photos: await listPhotos(env),
				photoBase: env.PHOTO_BASE_URL,
				thumbBase: env.THUMB_BASE_URL,
				repository: env.GITHUB_REPO,
				branch: env.GITHUB_BRANCH,
				canPublish: Boolean(env.GITHUB_TOKEN),
			});
		case "PUT /api/tags": {
			const body = (await request.json().catch(() => null)) as { tags?: unknown } | null;
			return json({ key: key(), tags: await setTags(env, key(), body?.tags) });
		}
		case "GET /api/original":
			return original(env, key());
		case "PUT /api/thumbnail":
			return json(await putThumbnail(env, key(), request));
		case "GET /api/config":
			return new Response((await draft(env)).text, {
				headers: {
					"Content-Type": "application/json; charset=utf-8",
					"Content-Disposition": 'attachment; filename="config.json"',
				},
			});
		case "POST /api/publish":
			return json(await publish(env));
	}

	if (url.pathname.startsWith("/api/")) throw new HttpError(404, "没有这个接口。");
	return env.ASSETS.fetch(request);
}

/**
 * Decides whether a request may go ahead. Everything here, pages included,
 * needs the admin token as the password of HTTP Basic authentication; the
 * user name is ignored.
 */
async function refuse(request: Request, env: Env): Promise<Response | null> {
	const token = env.ADMIN_TOKEN ?? "";
	if (token.length < 16) {
		// Never fall back to being open.
		return new Response("ADMIN_TOKEN 还没有设置，或者不足 16 个字符。", { status: 503 });
	}

	if (!(await sameText(password(request), token))) {
		return new Response("请用 ADMIN_TOKEN 作为密码登录。", {
			status: 401,
			headers: { "WWW-Authenticate": 'Basic realm="Photo admin", charset="UTF-8"' },
		});
	}

	// The browser attaches the saved password to requests that other sites
	// make to this one, so anything that changes data must come from our own
	// pages.
	if (request.method !== "GET" && request.method !== "HEAD" && !fromOwnPage(request)) {
		return new Response("不接受来自其他网站的请求。", { status: 403 });
	}
	return null;
}

function password(request: Request): string {
	const [scheme, value] = (request.headers.get("Authorization") ?? "").split(" ");
	if (scheme?.toLowerCase() !== "basic" || !value) return "";
	try {
		const bytes = Uint8Array.from(atob(value), (c) => c.charCodeAt(0));
		const credentials = new TextDecoder().decode(bytes);
		return credentials.slice(credentials.indexOf(":") + 1);
	} catch {
		return "";
	}
}

/** Compares two strings without revealing through timing where they differ. */
async function sameText(a: string, b: string): Promise<boolean> {
	const encoder = new TextEncoder();
	const [x, y] = await Promise.all([
		crypto.subtle.digest("SHA-256", encoder.encode(a)),
		crypto.subtle.digest("SHA-256", encoder.encode(b)),
	]);
	return crypto.subtle.timingSafeEqual(x, y);
}

function fromOwnPage(request: Request): boolean {
	const site = request.headers.get("Sec-Fetch-Site");
	if (site) return site === "same-origin";
	// Not a browser new enough to say, or not a browser at all.
	const origin = request.headers.get("Origin");
	return !origin || origin === new URL(request.url).origin;
}

function harden(response: Response, env: Env): Response {
	const images = [env.THUMB_BASE_URL, env.PHOTO_BASE_URL]
		.map((base) => (URL.canParse(base) ? new URL(base).origin : ""))
		.join(" ");
	const out = new Response(response.body, response);
	out.headers.set(
		"Content-Security-Policy",
		`default-src 'none'; script-src 'self'; worker-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' blob: ${images}; base-uri 'none'; form-action 'none'; frame-ancestors 'none'`,
	);
	out.headers.set("X-Content-Type-Options", "nosniff");
	out.headers.set("Referrer-Policy", "no-referrer");
	if (!out.headers.has("Cache-Control")) out.headers.set("Cache-Control", "no-store");
	return out;
}

function json(data: unknown, status = 200): Response {
	return Response.json(data, { status });
}
