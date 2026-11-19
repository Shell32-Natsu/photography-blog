// The admin page: browse the photos in the bucket, tag them, make missing
// thumbnails and publish the albums.

const $ = (selector) => document.querySelector(selector);
const el = (tag, props = {}, ...children) => {
	const node = Object.assign(document.createElement(tag), props);
	node.append(...children);
	return node;
};

const PAGE = 90; // cards added to the page at a time
const TAG_TYPE = "application/x-photo-tag"; // marks a tag being dragged
const IMAGE = /\.(jpe?g|png|gif|webp)$/i;

const state = {
	photos: [], // as the server lists them, in key order
	view: "all", // "all", "untagged", "nothumb" or { tag }
	query: "",
	sort: "name",
	selected: new Set(), // keys
	photoBase: "",
	thumbBase: "",
	repository: "",
	branch: "",
	canPublish: false,
	user: null, // the address of whoever signed in through Cloudflare Access
};

const cards = new Map(); // key → the card on the page
let listed = []; // the photos of the current view, in order
let lastPicked = -1; // index in `listed`, for shift-click

/* ── talking to the server ── */

async function api(path, init) {
	const response = await fetch(path, init);
	const body = await response.json().catch(() => null);
	if (!response.ok) throw new Error(body?.error ?? `请求失败（HTTP ${response.status}）`);
	return body;
}

async function load() {
	const data = await api("/api/photos");
	Object.assign(state, data);
	const keys = new Set(state.photos.map((photo) => photo.key));
	for (const key of state.selected) if (!keys.has(key)) state.selected.delete(key);
	render();
}

// At most four requests at a time: changing tags rewrites the whole photo.
const waiting = [];
let running = 0;
function limited(task) {
	return new Promise((resolve, reject) => {
		const start = () => {
			running++;
			task().then(resolve, reject).finally(() => {
				running--;
				waiting.shift()?.();
			});
		};
		running < 4 ? start() : waiting.push(start);
	});
}

/* ── tags ── */

const saves = new Map(); // key → the last save queued for that photo
let resync = false;

/** Gives a photo new tags: shown at once, saved in the background, in order. */
function retag(photo, tags) {
	if (tags.length == photo.tags.length && tags.every((tag, i) => tag == photo.tags[i])) return;
	photo.tags = tags;
	photo.saving = (photo.saving ?? 0) + 1;
	paint(photo);
	renderSide();

	const save = (saves.get(photo.key) ?? Promise.resolve())
		.then(() =>
			limited(() =>
				api("/api/tags?key=" + encodeURIComponent(photo.key), {
					method: "PUT",
					headers: { "Content-Type": "application/json" },
					body: JSON.stringify({ tags }),
				}),
			),
		)
		.catch((err) => {
			toast(`${photo.key}：${err.message}`, { error: true });
			resync = true;
		})
		.finally(() => {
			photo.saving--;
			paint(photo);
			if (saves.get(photo.key) == save) saves.delete(photo.key);
			// What is on screen may no longer be what is stored: ask again.
			if (resync && !saves.size) {
				resync = false;
				load().catch((err) => toast(err.message, { error: true }));
			}
		});
	saves.set(photo.key, save);
}

const parse = (text) => [...new Set(text.split(/[,，]/).map((tag) => tag.trim()).filter(Boolean))];
const add = (photo, tags) => retag(photo, [...new Set([...photo.tags, ...tags])]);
const remove = (photo, tags) => retag(photo, photo.tags.filter((tag) => !tags.includes(tag)));

/** The photos an action on one card applies to: all selected ones if it is among them. */
function targets(photo) {
	if (!state.selected.has(photo.key)) return [photo];
	return state.photos.filter((other) => state.selected.has(other.key));
}

/* ── rendering ── */

function render() {
	renderSide();
	renderGrid();
	renderBar();
}

function renderSide() {
	const counts = new Map();
	for (const photo of state.photos) for (const tag of photo.tags) counts.set(tag, (counts.get(tag) ?? 0) + 1);
	const tags = [...counts.keys()].sort((a, b) => a.localeCompare(b, "zh-Hans-CN"));

	const item = (view, label, count, draggable) => {
		const active = JSON.stringify(view) == JSON.stringify(state.view);
		const button = el("button", { type: "button", className: "pick" + (active ? " on" : ""), draggable }, label, el("span", {}, String(count)));
		button.onclick = () => {
			state.view = view;
			renderSide();
			renderGrid();
		};
		if (draggable) {
			button.ondragstart = (e) => {
				e.dataTransfer.setData(TAG_TYPE, label);
				e.dataTransfer.setData("text/plain", label);
				e.dataTransfer.effectAllowed = "copy";
			};
		}
		return el("li", {}, button);
	};

	$("#views").replaceChildren(
		item("all", "全部照片", state.photos.length),
		item("untagged", "未打标签", state.photos.filter((photo) => !photo.tags.length).length),
		item("nothumb", "缺缩略图", state.photos.filter((photo) => !photo.thumb).length),
	);
	$("#tags").replaceChildren(...tags.map((tag) => item({ tag }, tag, counts.get(tag), true)));
	$("#tag-count").textContent = tags.length;
	$("#known-tags").replaceChildren(...tags.map((tag) => el("option", { value: tag })));
	renderBar();
}

function renderBar() {
	const missing = state.photos.filter((photo) => !photo.thumb && IMAGE.test(photo.key)).length;
	const thumbs = $("#thumbs");
	if (!thumbs.disabled) {
		thumbs.hidden = !missing;
		thumbs.textContent = `生成缩略图（${missing}）`;
	}
	const publish = $("#publish");
	publish.disabled = !state.canPublish;
	publish.title = state.canPublish ? `提交 config.json 到 ${state.repository} 的 ${state.branch} 分支` : "还没有设置 GITHUB_TOKEN，可以先下载 config.json 手动提交";

	$("#user").hidden = !state.user;
	$("#user-name").textContent = state.user ?? "";

	$("#bulk").hidden = !state.selected.size;
	$("#bulk-count").textContent = `已选 ${state.selected.size} 张`;
}

function renderGrid() {
	const view = state.view;
	const query = state.query.trim().toLowerCase();
	listed = state.photos.filter(
		(photo) =>
			(view == "all" ||
				(view == "untagged" && !photo.tags.length) ||
				(view == "nothumb" && !photo.thumb) ||
				(view.tag && photo.tags.includes(view.tag))) &&
			photo.key.toLowerCase().includes(query),
	);
	if (state.sort == "new") listed.sort((a, b) => b.uploaded.localeCompare(a.uploaded));

	cards.clear();
	lastPicked = -1;
	$("#grid").replaceChildren();
	$("#status").hidden = listed.length > 0;
	$("#status").textContent = state.photos.length ? "没有符合条件的照片。" : "bucket 里还没有照片。";
	more();
}

/** Adds the next page of cards; called again as the end of the grid scrolls into view. */
function more() {
	const next = listed.slice(cards.size, cards.size + PAGE);
	$("#grid").append(...next.map(card));
}

new IntersectionObserver((entries) => entries.some((entry) => entry.isIntersecting) && more(), {
	rootMargin: "800px",
}).observe($("#more"));

const address = (base, key) => base.replace(/\/+$/, "") + "/" + key.split("/").map(encodeURIComponent).join("/");

function card(photo) {
	const node = $("#card").content.firstElementChild.cloneNode(true);
	node.dataset.key = photo.key;
	node.querySelector(".card-name").textContent = photo.key;
	node.querySelector(".card-name").title = photo.key;
	node.querySelector(".card-photo").href = address(state.photoBase, photo.key);
	node.querySelector("img").onerror = () => node.querySelector(".card-photo").classList.add("none");
	cards.set(photo.key, node);
	paint(photo);
	return node;
}

/** Brings a card up to date with its photo. */
function paint(photo) {
	const node = cards.get(photo.key);
	if (!node) return;
	node.classList.toggle("saving", Boolean(photo.saving));
	node.classList.toggle("picked", state.selected.has(photo.key));
	node.querySelector(".card-pick").checked = state.selected.has(photo.key);

	const frame = node.querySelector(".card-photo");
	const img = node.querySelector("img");
	const src = photo.thumb ? address(state.thumbBase, photo.key) + (photo.fresh ? "?" + photo.fresh : "") : "";
	frame.classList.toggle("none", !src);
	if (src && img.getAttribute("src") != src) img.src = src;

	node.querySelector(".chips").replaceChildren(
		...photo.tags.map((tag) =>
			el("li", { className: "chip" }, tag, el("button", { type: "button", title: `移除「${tag}」`, textContent: "×", value: tag })),
		),
	);
}

const photoOf = (node) => state.photos.find((photo) => photo.key == node.closest(".card")?.dataset.key);

/* ── the grid: tagging and selecting ── */

const grid = $("#grid");

grid.addEventListener("click", (e) => {
	const photo = photoOf(e.target);
	if (!photo) return;
	if (e.target.matches(".chip button")) {
		for (const target of targets(photo)) remove(target, [e.target.value]);
	}
	if (e.target.matches(".card-pick")) {
		const at = listed.indexOf(photo);
		const range = e.shiftKey && lastPicked >= 0 ? listed.slice(Math.min(at, lastPicked), Math.max(at, lastPicked) + 1) : [photo];
		for (const each of range) {
			e.target.checked ? state.selected.add(each.key) : state.selected.delete(each.key);
			paint(each);
		}
		lastPicked = at;
		renderBar();
	}
});

grid.addEventListener("keydown", (e) => {
	if (e.key != "Enter" || !e.target.matches(".card-add")) return;
	const photo = photoOf(e.target);
	const tags = parse(e.target.value);
	if (!photo || !tags.length) return;
	for (const target of targets(photo)) add(target, tags);
	e.target.value = "";
});

// A tag dragged from the side list lands on a card.
const dropTarget = (e) => (e.dataTransfer.types.includes(TAG_TYPE) ? e.target.closest(".card") : null);
grid.addEventListener("dragover", (e) => {
	const node = dropTarget(e);
	if (!node) return;
	e.preventDefault();
	e.dataTransfer.dropEffect = "copy";
	node.classList.add("drop");
});
grid.addEventListener("dragleave", (e) => e.target.closest(".card")?.classList.remove("drop"));
grid.addEventListener("drop", (e) => {
	const node = dropTarget(e);
	if (!node) return;
	e.preventDefault();
	node.classList.remove("drop");
	for (const target of targets(photoOf(node))) add(target, [e.dataTransfer.getData(TAG_TYPE)]);
});

/* ── the bar above the grid ── */

$("#search").addEventListener("input", (e) => {
	state.query = e.target.value;
	renderGrid();
});

$("#sort").addEventListener("change", (e) => {
	state.sort = e.target.value;
	renderGrid();
});

$("#bulk").addEventListener("submit", (e) => {
	e.preventDefault();
	const tags = parse($("#bulk-tag").value);
	if (!tags.length) return;
	const change = e.submitter?.value == "remove" ? remove : add;
	for (const photo of state.photos) if (state.selected.has(photo.key)) change(photo, tags);
	$("#bulk-tag").value = "";
});

function select(photos) {
	state.selected = new Set(photos.map((photo) => photo.key));
	for (const photo of listed) paint(photo);
	renderBar();
}
$("#bulk-all").onclick = () => select(listed);
$("#bulk-none").onclick = () => select([]);

/* ── thumbnails ── */

$("#thumbs").onclick = async () => {
	const button = $("#thumbs");
	const todo = state.photos.filter((photo) => !photo.thumb && IMAGE.test(photo.key));
	const worker = new Worker("/thumbnail.js");
	const failures = [];
	button.disabled = true;
	try {
		for (const [i, photo] of todo.entries()) {
			button.textContent = `正在生成缩略图 ${i + 1}/${todo.length}…`;
			const result = await new Promise((resolve) => {
				worker.onmessage = (e) => resolve(e.data);
				worker.onerror = (e) => resolve({ ok: false, error: e.message });
				worker.postMessage({ key: photo.key });
			});
			if (result.ok) {
				photo.thumb = true;
				photo.fresh = Date.now(); // gets past a cached "not found"
				paint(photo);
			} else {
				failures.push(`${photo.key}：${result.error}`);
			}
		}
	} finally {
		worker.terminate();
		button.disabled = false;
		renderSide();
	}
	if (failures.length) toast(`${failures.length} 张没有生成：\n${failures.join("\n")}`, { error: true });
	else toast(`已生成 ${todo.length} 张缩略图。`);
};

/* ── publishing ── */

$("#publish").onclick = async () => {
	if (!confirm(`按当前标签生成 config.json，并提交到 ${state.repository} 的 ${state.branch} 分支？`)) return;
	const button = $("#publish");
	button.disabled = true;
	try {
		await Promise.all(saves.values()); // let tag changes land first
		const result = await api("/api/publish", { method: "POST" });
		const summary = `${result.albums} 个相册，${result.photos} 张照片`;
		if (result.changed) toast(`已发布：${summary}。`, { link: result.commit, linkText: "查看提交" });
		else toast(`和已发布的内容一致，无需提交（${summary}）。`);
	} catch (err) {
		toast(err.message, { error: true });
	} finally {
		renderBar();
	}
};

/* ── messages ── */

function toast(message, { error = false, link, linkText } = {}) {
	const node = el("div", { className: "toast" + (error ? " toast-error" : ""), role: error ? "alert" : "status" }, message);
	if (link) node.append(" ", el("a", { href: link, target: "_blank", rel: "noopener", textContent: linkText }));
	node.append(el("button", { type: "button", title: "关闭", textContent: "×", onclick: () => node.remove() }));
	$("#toasts").append(node);
	if (!error) setTimeout(() => node.remove(), 8000);
}

load().catch((err) => {
	$("#status").textContent = `加载失败：${err.message}`;
});
