import { initElements, getEl, setPackHint } from "./notwailsjs/dom.js";
import {
	displayStaged,
	setStaged,
	selectedProfileIDs,
	browseDraftValues,
	getPackForEnable,
	getCatalogueSnap,
	setCatalogueSnap,
} from "./notwailsjs/state.js";
import { STATUS_POLL_MS, CERT_POLL_MS } from "./notwailsjs/constants.js";
import {
	refreshBrowseTable,
	buildDraftFromBrowseInputs,
} from "./notwailsjs/browse.js";
import {
	refreshInstalledPacks,
	handleUrlImport,
	handleFileImport,
	handleExport,
} from "./notwailsjs/pack.js";
import {
	updateMegiddoControl,
	applyCertPatchPill,
	refreshCertPatchPill,
	refreshLiveStatus,
} from "./notwailsjs/status.js";
import {
	EnableMegiddo,
	EnableMegiddoProfiles,
	DisableMegiddo,
	IsMegiddoEnabled,
	MergeInstalledMgpacks,
	GetRobloxCertPatchStatus,
	PatchRobloxCerts,
	UnpatchRobloxCerts,
	ClearRobloxCache,
	GetCatalogue,
} from "./wailsjs/go/main/App.js";

export async function ensureCatalogue() {
	let snap = getCatalogueSnap();
	if (!snap) {
		snap = await GetCatalogue();
		setCatalogueSnap(snap);
	}
	return snap;
}

export function activateTab(which) {
	document.querySelectorAll(".tabs .tab").forEach((b) => {
		if (b instanceof HTMLButtonElement)
			b.setAttribute("aria-selected", String(b.dataset.tab === which));
	});
	document.querySelectorAll(".tabpanel").forEach((el) => {
		const on = el.id === `panel-${which}`;
		el.toggleAttribute("hidden", !on);
		el.classList.toggle("hidden", !on);
	});
	if (which === "browse") refreshBrowseTable().catch(console.error);
}

async function init() {
	initElements();

	document.querySelectorAll(".tabs .tab").forEach((btn) => {
		btn.addEventListener("click", () =>
			activateTab(btn.dataset.tab || "home"),
		);
	});

	getEl("btnUrl")?.addEventListener("click", handleUrlImport);
	getEl("btnFile")?.addEventListener("click", handleFileImport);

	const btnMegiddo = getEl("btnMegiddo");
	if (btnMegiddo) {
		btnMegiddo.addEventListener("click", async () => {
			if (await IsMegiddoEnabled()) {
				await DisableMegiddo();
				updateMegiddoControl(false);
				return;
			}
			const target = getPackForEnable();
			if (target.mode === "profiles") {
				await EnableMegiddoProfiles(target.ids);
				const merged = await MergeInstalledMgpacks(target.ids);
				if (merged) setStaged(merged, `profiles(${target.ids.length})`);
			} else {
				await EnableMegiddo(target.pack);
				setStaged(target.pack);
			}
			displayStaged();
			updateMegiddoControl(true);
		});
	}

	getEl("btnExport")?.addEventListener("click", handleExport);
	getEl("btnStageBrowse")?.addEventListener("click", async () => {
		const snap = await ensureCatalogue();
		setStaged(buildDraftFromBrowseInputs(snap), "browse");
		selectedProfileIDs.clear();
		getEl("installedPacksEl")
			?.querySelectorAll('input[type="checkbox"]')
			.forEach((cb) => (cb.checked = false));
		displayStaged();
		activateTab("home");
	});
	getEl("btnBrowseReset")?.addEventListener("click", () => {
		browseDraftValues.clear();
		refreshBrowseTable().catch(console.error);
	});

	getEl("btnPatchCerts")?.addEventListener("click", async () => {
		const before = await GetRobloxCertPatchStatus();
		if (before.unpatched === 0) return;
		applyCertPatchPill(await PatchRobloxCerts());
	});
	getEl("btnUnpatchCerts")?.addEventListener("click", async () => {
		applyCertPatchPill(await UnpatchRobloxCerts());
	});
	getEl("btnClearRbxCache")?.addEventListener("click", async () => {
		const msg = (await ClearRobloxCache()) || "cache cleared";
		const wasOn = await IsMegiddoEnabled();
		if (wasOn) await DisableMegiddo();
		updateMegiddoControl(false);
		setPackHint(wasOn ? `${msg} - megiddo off` : msg);
	});
	getEl("btnLogsCopy")?.addEventListener("click", async () => {
		await navigator.clipboard.writeText(
			getEl("logFull")?.textContent || "",
		);
	});
	getEl("btnLogsRefresh")?.addEventListener("click", () =>
		refreshLiveStatus().catch(() => {}),
	);

	activateTab("home");
	displayStaged();
	await refreshInstalledPacks();
	await refreshLiveStatus();
	await refreshCertPatchPill();

	setInterval(() => refreshLiveStatus().catch(() => {}), STATUS_POLL_MS);
	setInterval(() => refreshCertPatchPill().catch(() => {}), CERT_POLL_MS);
}

document.body.addEventListener("click", (e) => {
	const target = e.target.closest("a[href^='http']");
	if (!target) return;
	e.preventDefault();
	window.runtime.BrowserOpenURL(target.href);
});

init().catch(console.error);
