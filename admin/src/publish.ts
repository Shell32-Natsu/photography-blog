import { buildConfig, serialize, type SiteConfig } from "./config";
import { HttpError, type Env } from "./index";
import { listPhotos } from "./photos";

const FILE = "config.json";

interface Draft {
	/** The text config.json should now have. */
	text: string;
	/** Whether that differs from what is in the repository. */
	changed: boolean;
	albums: number;
	photos: number;
	/** GitHub's identifier of the current file, needed to replace it. */
	sha?: string;
}

/** Works out config.json from the current tags, next to the published one. */
export async function draft(env: Env): Promise<Draft> {
	const [photos, current] = await Promise.all([listPhotos(env), env.GITHUB_TOKEN ? viaApi(env) : viaRaw(env)]);

	let previous: Partial<SiteConfig> | null = null;
	if (current.text !== null) {
		try {
			previous = JSON.parse(current.text) as Partial<SiteConfig>;
		} catch {
			throw new HttpError(502, `${env.GITHUB_REPO} 里的 ${FILE} 不是合法的 JSON，请先在仓库里修好。`);
		}
	}

	const config = buildConfig(photos, previous, env.PHOTO_BASE_URL);
	const text = serialize(config);
	return {
		text,
		changed: text !== current.text,
		albums: config.children.length,
		photos: new Set(config.children.flatMap((album) => album.children.map((photo) => photo.url))).size,
		sha: current.sha,
	};
}

/** Commits config.json to the repository, which sets off the site's deployment. */
export async function publish(env: Env): Promise<{ changed: boolean; albums: number; photos: number; commit?: string }> {
	if (!env.GITHUB_TOKEN) {
		throw new HttpError(400, "发布需要先设置 GITHUB_TOKEN；没有的话可以下载 config.json 后自己提交。");
	}
	const { text, changed, albums, photos, sha } = await draft(env);
	if (!changed) return { changed, albums, photos };

	const response = await github(env, {
		method: "PUT",
		body: JSON.stringify({
			message: "Update albums from photo tags",
			branch: env.GITHUB_BRANCH,
			content: toBase64(new TextEncoder().encode(text)),
			sha,
		}),
	});
	const result = (await response.json()) as { commit?: { html_url?: string } };
	return { changed, albums, photos, commit: result.commit?.html_url };
}

/** Reads the published file through the GitHub API, which also tells us its sha. */
async function viaApi(env: Env): Promise<{ text: string | null; sha?: string }> {
	const response = await github(env, { method: "GET" }, [404]);
	if (response.status === 404) return { text: null };
	const file = (await response.json()) as { sha: string; content?: string; encoding?: string };
	if (file.encoding === "base64" && file.content) {
		return { text: new TextDecoder().decode(fromBase64(file.content)), sha: file.sha };
	}
	// Files over 1 MB come without their content; ask for it separately.
	const raw = await github(env, { method: "GET", headers: { Accept: "application/vnd.github.raw+json" } });
	return { text: await raw.text(), sha: file.sha };
}

/** Reads the published file anonymously, which works for a public repository. */
async function viaRaw(env: Env): Promise<{ text: string | null; sha?: string }> {
	const base = env.GITHUB_RAW_URL ?? "https://raw.githubusercontent.com";
	const response = await fetch(`${base}/${env.GITHUB_REPO}/${encodeURIComponent(env.GITHUB_BRANCH)}/${FILE}`);
	if (response.status === 404) return { text: null };
	if (!response.ok) throw new HttpError(502, `从 GitHub 读取 ${FILE} 失败（HTTP ${response.status}）。`);
	return { text: await response.text() };
}

async function github(env: Env, init: RequestInit, allow: number[] = []): Promise<Response> {
	const base = env.GITHUB_API_URL ?? "https://api.github.com";
	const url = new URL(`${base}/repos/${env.GITHUB_REPO}/contents/${FILE}`);
	if (init.method === "GET") url.searchParams.set("ref", env.GITHUB_BRANCH);

	const response = await fetch(url, {
		...init,
		headers: {
			Accept: "application/vnd.github+json",
			Authorization: `Bearer ${env.GITHUB_TOKEN}`,
			"X-GitHub-Api-Version": "2022-11-28",
			"User-Agent": "photography-blog-admin",
			...(init.headers as Record<string, string> | undefined),
		},
	});
	if (!response.ok && !allow.includes(response.status)) {
		const detail = ((await response.json().catch(() => null)) as { message?: string } | null)?.message;
		throw new HttpError(502, `GitHub 拒绝了请求（HTTP ${response.status}${detail ? `：${detail}` : ""}）。`);
	}
	return response;
}

function toBase64(bytes: Uint8Array): string {
	let binary = "";
	for (let i = 0; i < bytes.length; i += 0x8000) {
		binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
	}
	return btoa(binary);
}

function fromBase64(text: string): Uint8Array {
	return Uint8Array.from(atob(text.replace(/\s/g, "")), (c) => c.charCodeAt(0));
}
