// The files in pages/ are imported as text (see "rules" in wrangler.jsonc).

declare module "*.html" {
	const text: string;
	export default text;
}

declare module "*.css" {
	const text: string;
	export default text;
}
