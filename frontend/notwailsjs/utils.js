export function packLabel(p) {
	if (!p) return "no pack";
	const name = String(p.name || "").trim() || "unnamed";
	const author = String(p.author || "").trim();
	return author ? `${name} (${author})` : name;
}

export function roleLabel(role) {
	const key = String(role || "").toLowerCase();
	if (key === "alter_ego") return "Alter Ego";
	if (key === "victim") return "Victim";
	if (key === "ui") return "UI";
	return String(role).replace(/_/g, " ");
}

export function rowCharacter(row) {
	return typeof row?.character === "string" && row.character.trim()
		? row.character.trim()
		: "(misc)";
}

/** Normalized catalogue/pack role from the row's `role` field. */
export function rowRole(row) {
	const r = String(row?.role ?? "")
		.trim()
		.toLowerCase();
	if (r === "alter_ego" || r === "victim" || r === "ui") return r;
	return "victim";
}

export function rowModel(row) {
	const m = row?.model;
	if (typeof m === "string" && m.trim()) return m.trim();
	const path = row?.rbxm_props?.instance_path;
	if (typeof path === "string" && path.includes(".")) {
		const seg = path.split(".")[1];
		if (seg) return seg;
	}
	return "Default";
}

export function sortSkins(skins) {
	return skins.sort((a, b) => {
		if (a === "Default") return -1;
		if (b === "Default") return 1;
		return a.localeCompare(b);
	});
}

export function skinsForCharacter(rows, character) {
	const set = new Set();
	for (const r of rows)
		if (rowCharacter(r) === character) set.add(rowModel(r));
	return sortSkins([...set]);
}

export function pickDefaultSkin(rows, character) {
	const skins = skinsForCharacter(rows, character);
	return skins.includes("Default") ? "Default" : skins[0] || "Default";
}

export function coerceSlot(row) {
	const at = String(row.asset_type ?? "").toLowerCase();
	if (at !== "texturepack" && !at.includes("texturepack")) return null;
	if (typeof row.slot === "number" && Number.isFinite(row.slot))
		return row.slot;
	if (row.slot != null) {
		const s = Number(row.slot);
		if (Number.isFinite(s) && s >= 0) return s;
	}
	return null;
}

export function replacementMatchKey(targetID, slot, assetType) {
	const s = slot != null ? slot : -1;
	return `${targetID}:${s}:${String(assetType || "").toLowerCase()}`;
}

export function parsePositiveId(raw) {
	const v = raw.trim();
	if (!v) return null;
	const n = Number(v);
	if (!Number.isFinite(n) || n <= 0 || !Number.isInteger(n)) return null;
	return n;
}

export function parseReplaceWith(raw) {
	const v = raw.trim();
	if (!v) return null;
	const n = Number(v);
	if (!Number.isFinite(n) || n < 0 || !Number.isInteger(n)) return null;
	return n;
}

export function canClear(assetType) {
	const at = String(assetType ?? "").toLowerCase();
	return (
		at === "texturepack" ||
		at === "texture" ||
		at === "image" ||
		at === "decal" ||
		at === "mesh"
	);
}
