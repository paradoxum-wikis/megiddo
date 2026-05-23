import { getEl, setPackHint } from "./dom.js";
import { packLabel } from "./utils.js";
import { syncMegiddoFromBackend } from "./status.js";

const appState = { staged: null, stagedSource: "" };
export const selectedProfileIDs = new Set();
export const browseDraftValues = new Map();

export let selectedBrowseRole = "";
export let selectedBrowseCharacter = "";
export let selectedBrowseModel = "";

export function setSelectedBrowseRole(v) {
	selectedBrowseRole = v;
}
export function setSelectedBrowseCharacter(v) {
	selectedBrowseCharacter = v;
}
export function setSelectedBrowseModel(v) {
	selectedBrowseModel = v;
}

export function getStaged() {
	return appState.staged;
}
export function getStagedSource() {
	return appState.stagedSource;
}
export function setStaged(p, source = "") {
	appState.staged = p;
	appState.stagedSource = source;
}
export function clearStaged() {
	appState.staged = null;
	appState.stagedSource = "";
}

let catalogueSnap = null;
export function getCatalogueSnap() {
	return catalogueSnap;
}
export function setCatalogueSnap(s) {
	catalogueSnap = s;
}

export function syncPreviewFromStaged() {
	const el = getEl("preview");
	if (el)
		el.value = appState.staged
			? JSON.stringify(appState.staged, null, 2)
			: "";
}

export function trySyncStagedFromPreview() {
	const el = getEl("preview");
	if (!el) return false;
	const raw = el.value.trim();
	if (!raw) return false;
	try {
		const p = JSON.parse(raw);
		if (!Array.isArray(p.replacements) || p.replacements.length === 0)
			return false;
		appState.staged = p;
		return true;
	} catch {
		return false;
	}
}

export function getPackForEnable() {
	const sel = Array.from(selectedProfileIDs);
	if (sel.length) return { mode: "profiles", ids: sel };
	if (appState.staged) return { mode: "pack", pack: appState.staged };
	if (trySyncStagedFromPreview())
		return { mode: "pack", pack: appState.staged };
	throw new Error(
		"import a pack, check saved packs, or stage a browse draft first",
	);
}

export function displayStaged() {
	syncPreviewFromStaged();
	if (!appState.staged) {
		setPackHint("import a pack or check saved packs, then enable Megiddo.");
		syncMegiddoFromBackend().catch(console.error);
		return;
	}
	const src = appState.stagedSource ? ` - ${appState.stagedSource}` : "";
	setPackHint(`ready: ${packLabel(appState.staged)}${src}`);
	syncMegiddoFromBackend().catch(console.error);
}

export function populateMetaFromPack(p) {
	const map = {
		metaName: "name",
		metaAuthor: "author",
		metaVersion: "version",
		metaDescription: "description",
	};
	Object.entries(map).forEach(([id, key]) => {
		const el = getEl(id);
		if (el) el.value = String(p[key] || "");
	});
}
