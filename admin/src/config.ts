// config.json, the file the site is generated from: one album per tag.

export interface PhotoNode {
	url: string;
}

export interface AlbumNode {
	title: string;
	description?: string;
	/** The cover. */
	url: string;
	children: PhotoNode[];
}

export interface SiteConfig {
	title: string;
	description: string;
	author: string;
	children: AlbumNode[];
}

/**
 * Builds the configuration from the photos' tags. What tags cannot express
 * is carried over from the configuration already published: the site's own
 * title, description and author, and each album's description and cover.
 * An album whose cover is gone, or a new one, gets a cover picked by `pick`.
 */
export function buildConfig(
	photos: readonly { key: string; tags: readonly string[] }[],
	previous: Partial<SiteConfig> | null,
	photoBase: string,
	pick: (urls: string[]) => string = atRandom,
): SiteConfig {
	const base = photoBase.replace(/\/+$/, "");
	const albums = new Map<string, string[]>();
	for (const photo of photos) {
		for (const tag of photo.tags) {
			let urls = albums.get(tag);
			if (!urls) albums.set(tag, (urls = []));
			urls.push(`${base}/${photo.key}`);
		}
	}

	const before = new Map((previous?.children ?? []).map((album) => [album.title, album]));
	const children = [...albums.keys()].sort(byCodePoint).map((title): AlbumNode => {
		const urls = albums.get(title)!;
		const old = before.get(title);
		return {
			title,
			...(old?.description ? { description: old.description } : {}),
			url: old && urls.includes(old.url) ? old.url : pick(urls),
			children: urls.map((url) => ({ url })),
		};
	});

	return {
		title: previous?.title ?? "Photography Collection",
		description: previous?.description ?? "Photo albums by tag",
		author: previous?.author ?? "",
		children,
	};
}

/** The exact text of config.json: two-space indentation, no final newline. */
export function serialize(config: SiteConfig): string {
	return JSON.stringify(config, null, 2);
}

function atRandom(urls: string[]): string {
	return urls[Math.floor(Math.random() * urls.length)]!;
}

/**
 * Orders strings by Unicode code point, which is the order of their UTF-8
 * bytes and therefore the order R2 lists keys in.
 */
function byCodePoint(a: string, b: string): number {
	const x = Array.from(a, (c) => c.codePointAt(0)!);
	const y = Array.from(b, (c) => c.codePointAt(0)!);
	for (let i = 0; i < x.length && i < y.length; i++) {
		if (x[i] !== y[i]) return x[i]! - y[i]!;
	}
	return x.length - y.length;
}
