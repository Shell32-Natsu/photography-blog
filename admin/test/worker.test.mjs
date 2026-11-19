// Runs the Worker in the real Workers runtime with local, empty R2 buckets
// and a stand-in for GitHub: npm test

import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import { createServer } from "node:http";
import { after, afterEach, before, beforeEach, describe, test } from "node:test";
import { fileURLToPath } from "node:url";
import { createTestHarness } from "wrangler";

const TOKEN = "correct-horse-battery-staple";
const AUTH = { Authorization: "Basic " + Buffer.from("anyone:" + TOKEN).toString("base64") };
const JPEG = Buffer.concat([Buffer.from([0xff, 0xd8, 0xff, 0xe0]), Buffer.alloc(2000, 7)]);
const REPO = "Shell32-Natsu/photography-blog";

/** A stand-in for GitHub that serves and accepts config.json. */
const github = {
	file: null, // text of config.json in the "repository", or null if there is none
	sha: "sha-of-current-file",
	puts: [], // bodies of the commits received
	fail: 0, // answer everything with this status instead
	server: null,
	url: "",
	async start() {
		this.server = createServer(async (req, res) => {
			const send = (status, body) => {
				res.writeHead(status, { "Content-Type": "application/json" });
				res.end(typeof body === "string" ? body : JSON.stringify(body));
			};
			const url = new URL(req.url, "http://github.test");
			if (this.fail) return send(this.fail, { message: "Bad credentials" });

			if (url.pathname === `/${REPO}/master/config.json`) {
				return this.file === null ? send(404, "404: Not Found") : send(200, this.file);
			}
			if (url.pathname === `/repos/${REPO}/contents/config.json`) {
				if (req.headers.authorization !== "Bearer github-token") return send(401, { message: "Bad credentials" });
				if (req.method === "GET") {
					assert.equal(url.searchParams.get("ref"), "master");
					if (this.file === null) return send(404, { message: "Not Found" });
					// GitHub wraps the base64 text at 60 characters.
					const content = Buffer.from(this.file).toString("base64").replace(/(.{60})/g, "$1\n");
					return send(200, { sha: this.sha, encoding: "base64", content });
				}
				if (req.method === "PUT") {
					let body = "";
					for await (const chunk of req) body += chunk;
					this.puts.push(JSON.parse(body));
					return send(200, { commit: { html_url: "https://github.test/commit/abc123" } });
				}
			}
			send(404, { message: "Not Found" });
		});
		await new Promise((resolve) => this.server.listen(0, "127.0.0.1", resolve));
		this.url = `http://127.0.0.1:${this.server.address().port}`;
	},
	reset() {
		Object.assign(this, { file: null, sha: "sha-of-current-file", puts: [], fail: 0 });
	},
};

await github.start();

const worker = (secrets) => ({
	configPath: "./wrangler.jsonc",
	vars: { GITHUB_API_URL: github.url, GITHUB_RAW_URL: github.url },
	secrets,
});
const server = createTestHarness({
	root: fileURLToPath(new URL("..", import.meta.url)),
	workers: [worker({ ADMIN_TOKEN: TOKEN, GITHUB_TOKEN: "github-token" })],
});

let env;
const call = (path, init = {}) => server.fetch(path, { ...init, headers: { ...AUTH, ...init.headers } });
const setTags = (key, tags) =>
	call(`/api/tags?key=${encodeURIComponent(key)}`, { method: "PUT", body: JSON.stringify({ tags }) });
const photos = async () => (await (await call("/api/photos")).json()).photos;
const seed = (key, tag, bucket = env.PHOTOS) =>
	bucket.put(key, JPEG, { httpMetadata: { contentType: "image/jpeg" }, customMetadata: tag === undefined ? {} : { tag } });

before(async () => {
	await server.listen();
});
beforeEach(async () => {
	github.reset();
	env = await server.getWorker().getEnv();
});
afterEach(async () => {
	await server.reset(); // empties the buckets and restores the secrets
});
after(async () => {
	await server.close();
	github.server.close();
});

describe("signing in", () => {
	test("nothing is served without the token, pages included", async () => {
		for (const path of ["/", "/app.js", "/api/photos"]) {
			const response = await server.fetch(path);
			assert.equal(response.status, 401, path);
			assert.match(response.headers.get("WWW-Authenticate"), /^Basic /);
		}
		const wrong = { Authorization: "Basic " + Buffer.from("anyone:not-the-token").toString("base64") };
		assert.equal((await server.fetch("/api/photos", { headers: wrong })).status, 401);
		assert.equal((await server.fetch("/api/photos", { headers: { Authorization: "Bearer " + TOKEN } })).status, 401);
	});

	test("the token opens pages and API", async () => {
		const page = await call("/");
		assert.equal(page.status, 200);
		assert.match(page.headers.get("Content-Type"), /text\/html/);
		assert.match(await page.text(), /<script src="\/app\.js"/);
		assert.equal((await call("/api/photos")).status, 200);
		assert.equal((await call("/api/nope")).status, 404);
	});

	test("a missing or short token closes everything rather than opening it", async () => {
		for (const token of [undefined, "", "short"]) {
			await server.update({ workers: [worker(token === undefined ? {} : { ADMIN_TOKEN: token })] });
			const guess = { Authorization: "Basic " + Buffer.from("anyone:" + (token ?? "")).toString("base64") };
			assert.equal((await server.fetch("/api/photos", { headers: guess })).status, 503);
			assert.equal((await server.fetch("/", { headers: guess })).status, 503);
		}
	});

	test("other sites cannot change anything with the saved password", async () => {
		await seed("a.jpg", "x");
		const put = (headers) =>
			call("/api/tags?key=a.jpg", { method: "PUT", body: JSON.stringify({ tags: ["y"] }), headers });
		assert.equal((await put({ "Sec-Fetch-Site": "cross-site" })).status, 403);
		assert.equal((await put({ "Sec-Fetch-Site": "same-site" })).status, 403);
		assert.equal((await put({ Origin: "https://evil.example" })).status, 403);
		assert.deepEqual((await photos())[0].tags, ["x"]);

		assert.equal((await put({ "Sec-Fetch-Site": "same-origin" })).status, 200);
		assert.equal((await put({})).status, 200); // a script, not a browser
	});

	test("responses carry the security headers", async () => {
		const response = await call("/");
		const csp = response.headers.get("Content-Security-Policy");
		assert.match(csp, /default-src 'none'/);
		assert.match(csp, /img-src 'self' blob: https:\/\/photo-thumbs-r2\.xiadong\.info https:\/\/photo-r2\.xiadong\.info;/);
		assert.equal(response.headers.get("X-Content-Type-Options"), "nosniff");
		assert.equal((await call("/api/photos")).headers.get("Cache-Control"), "no-store");
	});
});

describe("photos", () => {
	test("lists every photo with its tags and whether it has a thumbnail", async () => {
		await seed("b.jpg", " 日本 , 城市,,日本 ");
		await seed("a b.jpg", undefined);
		await seed("c.jpg", "Napa");
		await seed("b.jpg", undefined, env.THUMBS);

		const body = await (await call("/api/photos")).json();
		assert.deepEqual(
			body.photos.map(({ key, tags, thumb, size }) => ({ key, tags, thumb, size })),
			[
				{ key: "a b.jpg", tags: [], thumb: false, size: JPEG.length },
				{ key: "b.jpg", tags: ["日本", "城市"], thumb: true, size: JPEG.length },
				{ key: "c.jpg", tags: ["Napa"], thumb: false, size: JPEG.length },
			],
		);
		assert.ok(!Number.isNaN(Date.parse(body.photos[0].uploaded)));
		assert.equal(body.photoBase, "https://photo-r2.xiadong.info");
		assert.equal(body.canPublish, true);
	});

	test("lists buckets larger than one page", async () => {
		await Promise.all(Array.from({ length: 1203 }, (_, i) => seed(`p${String(i).padStart(4, "0")}.jpg`, "x")));
		const all = await photos();
		assert.equal(all.length, 1203);
		assert.equal(all.at(-1).key, "p1202.jpg");
	});

	test("changing tags rewrites the photo without touching anything else", async () => {
		// the size of a real photo, so that it has to be streamed
		const original = Buffer.alloc(12 << 20);
		for (let i = 0; i < original.length; i += 4096) original.writeUInt32LE((i * 2654435761) >>> 0, i);
		await env.PHOTOS.put("DSCF 1.jpg", original, {
			httpMetadata: { contentType: "image/jpeg", cacheControl: "max-age=60" },
			customMetadata: { tag: "旧", camera: "X-S10" },
		});

		const response = await setTags("DSCF 1.jpg", [" 日本 ", "城市", "日本", ""]);
		assert.equal(response.status, 200);
		assert.deepEqual(await response.json(), { key: "DSCF 1.jpg", tags: ["日本", "城市"] });

		const stored = await env.PHOTOS.get("DSCF 1.jpg");
		assert.deepEqual(stored.customMetadata, { tag: "日本,城市", camera: "X-S10" });
		assert.equal(stored.httpMetadata.contentType, "image/jpeg");
		assert.equal(stored.httpMetadata.cacheControl, "max-age=60");
		const sha = (data) => createHash("sha256").update(Buffer.from(data)).digest("hex");
		assert.equal(sha(await stored.arrayBuffer()), sha(original));
		assert.deepEqual((await photos())[0].tags, ["日本", "城市"]);
	});

	test("two changes to one photo at once: neither is lost silently", async () => {
		await seed("a.jpg", "x");
		const [first, second] = await Promise.all([setTags("a.jpg", ["one"]), setTags("a.jpg", ["two"])]);
		const stored = (await env.PHOTOS.head("a.jpg")).customMetadata.tag;
		// Either they ran one after the other, or the later one was told to retry.
		const statuses = [first.status, second.status].sort();
		if (statuses[1] === 409) {
			assert.deepEqual(statuses, [200, 409]);
			assert.equal(stored, first.status === 200 ? "one" : "two");
		} else {
			assert.deepEqual(statuses, [200, 200]);
			assert.ok(["one", "two"].includes(stored));
		}
		assert.deepEqual(Buffer.from(await (await env.PHOTOS.get("a.jpg")).arrayBuffer()), JPEG);
	});

	test("removing the last tag removes the field", async () => {
		await seed("a.jpg", "x");
		assert.equal((await setTags("a.jpg", [])).status, 200);
		assert.deepEqual((await env.PHOTOS.head("a.jpg")).customMetadata, {});
	});

	test("refuses tags it could not store faithfully", async () => {
		await seed("a.jpg", "x");
		for (const tags of [["a,b"], ["x".repeat(65)], ["line\nbreak"], "日本", [1], null, Array.from({ length: 33 }, (_, i) => `t${i}`)]) {
			const response = await setTags("a.jpg", tags);
			assert.equal(response.status, 400, JSON.stringify(tags));
			assert.ok((await response.json()).error);
		}
		assert.equal((await call("/api/tags?key=a.jpg", { method: "PUT", body: "not json" })).status, 400);
		assert.deepEqual((await photos())[0].tags, ["x"]);

		assert.equal((await setTags("missing.jpg", ["x"])).status, 404);
		assert.equal((await call("/api/tags", { method: "PUT", body: "{}" })).status, 400);
	});

	test("hands out originals", async () => {
		await seed("a b.jpg", "x");
		const response = await call("/api/original?key=" + encodeURIComponent("a b.jpg"));
		assert.equal(response.status, 200);
		assert.equal(response.headers.get("Content-Type"), "image/jpeg");
		assert.deepEqual(Buffer.from(await response.arrayBuffer()), JPEG);
		assert.equal((await call("/api/original?key=missing.jpg")).status, 404);
	});

	test("stores thumbnails made by the page", async () => {
		await seed("a.jpg", "x");
		const put = (key, body) => call("/api/thumbnail?key=" + key, { method: "PUT", body });

		const response = await put("a.jpg", JPEG);
		assert.equal(response.status, 200);
		assert.deepEqual(await response.json(), { key: "a.jpg", size: JPEG.length });
		const stored = await env.THUMBS.get("a.jpg");
		assert.equal(stored.httpMetadata.contentType, "image/jpeg");
		assert.deepEqual(Buffer.from(await stored.arrayBuffer()), JPEG);
		assert.equal((await photos())[0].thumb, true);

		assert.equal((await put("a.jpg", Buffer.from("<svg onload=alert(1)>"))).status, 415);
		assert.equal((await put("a.jpg", Buffer.concat([JPEG, Buffer.alloc(9 << 20)]))).status, 413);
		assert.equal((await put("missing.jpg", JPEG)).status, 404);
	});
});

describe("publishing", () => {
	const config = async () => JSON.parse(await (await call("/api/config")).text());

	test("reproduces the config.json in this repository from its own tags", async () => {
		// The albums in config.json are exactly the tags the photos had when it
		// was generated, so tagging photos that way must give the same file back,
		// byte for byte.
		const published = await readFile(new URL("../../config.json", import.meta.url), "utf8");
		const tags = new Map();
		for (const album of JSON.parse(published).children) {
			for (const { url } of album.children) {
				const key = url.slice("https://photo-r2.xiadong.info/".length);
				tags.set(key, [...(tags.get(key) ?? []), album.title]);
			}
		}
		await Promise.all([...tags].map(([key, list]) => seed(key, list.join(","))));
		github.file = published;

		const response = await call("/api/config");
		assert.equal(response.headers.get("Content-Disposition"), 'attachment; filename="config.json"');
		assert.equal(await response.text(), published);

		assert.deepEqual(await (await call("/api/publish", { method: "POST" })).json(), {
			changed: false,
			albums: JSON.parse(published).children.length,
			photos: tags.size,
		});
		assert.equal(github.puts.length, 0);
	});

	test("commits the new file when tags have changed", async () => {
		github.file = JSON.stringify({
			title: "博客",
			description: "简介",
			author: "作者",
			children: [
				{ title: "日本", description: "樱花季", url: "https://photo-r2.xiadong.info/b.jpg", children: [] },
				{ title: "旧相册", url: "https://photo-r2.xiadong.info/gone.jpg", children: [] },
			],
		});
		await seed("a.jpg", "日本,Napa");
		await seed("b.jpg", "日本");
		await seed("c d.jpg", "Napa,云");
		await seed("untagged.jpg", undefined);

		const result = await (await call("/api/publish", { method: "POST" })).json();
		assert.deepEqual(result, { changed: true, albums: 3, photos: 3, commit: "https://github.test/commit/abc123" });

		assert.equal(github.puts.length, 1);
		const { content, ...commit } = github.puts[0];
		assert.deepEqual(commit, { message: "Update albums from photo tags", branch: "master", sha: "sha-of-current-file" });
		const text = Buffer.from(content, "base64").toString();
		assert.equal(text, JSON.stringify(JSON.parse(text), null, 2));

		const site = JSON.parse(text);
		assert.deepEqual([site.title, site.description, site.author], ["博客", "简介", "作者"]);
		assert.deepEqual(
			site.children.map((album) => album.title),
			["Napa", "云", "日本"],
		);
		const [napa, , japan] = site.children;
		assert.deepEqual(japan, {
			title: "日本",
			description: "樱花季",
			url: "https://photo-r2.xiadong.info/b.jpg", // the cover it already had
			children: [{ url: "https://photo-r2.xiadong.info/a.jpg" }, { url: "https://photo-r2.xiadong.info/b.jpg" }],
		});
		assert.deepEqual(napa.children, [
			{ url: "https://photo-r2.xiadong.info/a.jpg" },
			{ url: "https://photo-r2.xiadong.info/c d.jpg" },
		]);
		assert.ok(napa.children.some((photo) => photo.url === napa.url), "a new album's cover is one of its photos");
	});

	test("replaces a cover that left its album", async () => {
		github.file = JSON.stringify({
			title: "t",
			children: [{ title: "x", url: "https://photo-r2.xiadong.info/a.jpg", children: [] }],
		});
		await seed("a.jpg", "y");
		await seed("b.jpg", "x");
		const site = await config();
		assert.equal(site.children.find((album) => album.title === "x").url, "https://photo-r2.xiadong.info/b.jpg");
	});

	test("creates the file when the repository has none", async () => {
		await seed("a.jpg", "x");
		assert.equal((await (await call("/api/publish", { method: "POST" })).json()).changed, true);
		assert.equal(github.puts[0].sha, undefined);
		assert.equal(JSON.parse(Buffer.from(github.puts[0].content, "base64")).title, "Photography Collection");
	});

	test("without a GitHub token the file can be downloaded but not committed", async () => {
		await server.update({ workers: [worker({ ADMIN_TOKEN: TOKEN })] });
		env = await server.getWorker().getEnv();
		github.file = JSON.stringify({ title: "博客", description: "", author: "", children: [] });
		await seed("a.jpg", "x");

		assert.equal((await (await call("/api/photos")).json()).canPublish, false);
		assert.equal((await config()).title, "博客"); // read anonymously
		const response = await call("/api/publish", { method: "POST" });
		assert.equal(response.status, 400);
		assert.match((await response.json()).error, /GITHUB_TOKEN/);
		assert.equal(github.puts.length, 0);
	});

	test("reports what GitHub said when it refuses", async () => {
		await seed("a.jpg", "x");
		github.fail = 401;
		const response = await call("/api/publish", { method: "POST" });
		assert.equal(response.status, 502);
		assert.match((await response.json()).error, /401.*Bad credentials/);

		github.fail = 0;
		github.file = "{ not json";
		assert.equal((await call("/api/publish", { method: "POST" })).status, 502);
		assert.equal(github.puts.length, 0);
	});
});
