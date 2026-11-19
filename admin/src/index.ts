// The admin for the photography blog: tag the photos in R2, make their
// thumbnails, and publish the albums that the tags describe.

import styles from "../pages/app.css";
import script from "../pages/app.js";
import page from "../pages/index.html";
import thumbnailer from "../pages/thumbnail.js";
import { listPhotos, original, putThumbnail, setTags } from "./photos";
import { draft, publish } from "./publish";

export interface Env {
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

const PAGES: Record<string, [body: string, type: string]> = {
	"/": [page, "text/html; charset=utf-8"],
	"/app.css": [styles, "text/css; charset=utf-8"],
	"/app.js": [script, "text/javascript; charset=utf-8"],
	"/thumbnail.js": [thumbnailer, "text/javascript; charset=utf-8"],
};

export default {
	async fetch(request, env, ctx): Promise<Response> {
		let response: Response;
		try {
			response = refuse(request, ctx) ?? (await route(request, env, ctx));
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

/**
 * Decides whether a request may go ahead. Signing in is left to Cloudflare
 * Access, which sits in front of the Worker; the runtime vouches for a
 * request that came through it by setting `ctx.access`. Anything else is
 * turned away, pages included: if Access is not switched on for this Worker,
 * nobody gets in.
 */
function refuse(request: Request, ctx: ExecutionContext): Response | null {
	if (!ctx.access) {
		return new Response("这个后台需要通过 Cloudflare Access 登录；请确认已经为这个 Worker 开启 Access。", {
			status: 403,
			headers: { "Content-Type": "text/plain; charset=utf-8" },
		});
	}
	// The browser attaches the session to requests that other sites make to
	// this one, so anything that changes data must come from our own pages.
	if (request.method !== "GET" && request.method !== "HEAD" && !fromOwnPage(request)) {
		return new Response("不接受来自其他网站的请求。", {
			status: 403,
			headers: { "Content-Type": "text/plain; charset=utf-8" },
		});
	}
	return null;
}

function fromOwnPage(request: Request): boolean {
	const site = request.headers.get("Sec-Fetch-Site");
	if (site) return site === "same-origin";
	// Not a browser new enough to say, or not a browser at all.
	const origin = request.headers.get("Origin");
	return !origin || origin === new URL(request.url).origin;
}

async function route(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
	const url = new URL(request.url);
	const key = () => {
		const key = url.searchParams.get("key");
		if (!key) throw new HttpError(400, "缺少照片的 key。");
		return key;
	};

	switch (`${request.method} ${url.pathname}`) {
		case "GET /api/photos": {
			const [photos, identity] = await Promise.all([listPhotos(env), ctx.access?.getIdentity()]);
			return json({
				photos,
				photoBase: env.PHOTO_BASE_URL,
				thumbBase: env.THUMB_BASE_URL,
				repository: env.GITHUB_REPO,
				branch: env.GITHUB_BRANCH,
				canPublish: Boolean(env.GITHUB_TOKEN),
				user: identity?.email ?? null,
			});
		}
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

	const found = request.method === "GET" || request.method === "HEAD" ? PAGES[url.pathname] : undefined;
	if (found) {
		const [body, type] = found;
		return new Response(request.method === "HEAD" ? null : body, {
			headers: { "Content-Type": type, "Cache-Control": "no-cache" },
		});
	}
	if (url.pathname.startsWith("/api/")) throw new HttpError(404, "没有这个接口。");
	return new Response("没有这个页面。", { status: 404, headers: { "Content-Type": "text/plain; charset=utf-8" } });
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
