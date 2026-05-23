const elements = {};

export function initElements() {
	elements.chipElevated = document.querySelector("#chipElevated");
	elements.chipPatch = document.querySelector("#chipPatch");
	elements.chipLifecycle = document.querySelector("#chipLifecycle");
	elements.btnMegiddo = document.getElementById("btnMegiddo");
	elements.megiddoHelp = document.querySelector("#megiddoHelp");
	elements.preview = document.getElementById("preview");
	elements.browseBody = document.getElementById("browseBody");
	elements.browseRoleButtons = document.querySelector("#browseRoleButtons");
	elements.browseCharacterButtons = document.querySelector(
		"#browseCharacterButtons",
	);
	elements.browseSkinButtons = document.querySelector("#browseSkinButtons");
	elements.browseNavHint = document.querySelector("#browseNavHint");
	elements.logFull = document.getElementById("logFull");
	elements.previewErr = document.querySelector("#previewErr");
	elements.installedPacksEl = document.getElementById("installedPacks");
	elements.packHint = document.querySelector("#packHint");
	elements.metaName = document.getElementById("metaName");
	elements.metaAuthor = document.getElementById("metaAuthor");
	elements.metaVersion = document.getElementById("metaVersion");
	elements.metaDescription = document.getElementById("metaDescription");
	elements.packUrl = document.getElementById("packUrl");
	elements.btnUrl = document.getElementById("btnUrl");
	elements.btnFile = document.getElementById("btnFile");
	elements.btnExport = document.getElementById("btnExport");
	elements.btnStageBrowse = document.getElementById("btnStageBrowse");
	elements.btnBrowseReset = document.getElementById("btnBrowseReset");
	elements.btnPatchCerts = document.getElementById("btnPatchCerts");
	elements.btnUnpatchCerts = document.getElementById("btnUnpatchCerts");
	elements.btnClearRbxCache = document.getElementById("btnClearRbxCache");
	elements.btnLogsCopy = document.getElementById("btnLogsCopy");
	elements.btnLogsRefresh = document.getElementById("btnLogsRefresh");
	return elements;
}

export function getEl(key) {
	return elements[key];
}

export function setPackHint(txt) {
	const el = getEl("packHint");
	if (el) el.textContent = txt;
}

export function setPreviewErr(msg) {
	const el = getEl("previewErr");
	if (!el) return;
	el.textContent = msg || "";
	el.classList.toggle("hidden", !msg);
}
