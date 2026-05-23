import { getEl, setPackHint } from "./dom.js";
import {
	getStaged,
	setStaged,
	clearStaged,
	selectedProfileIDs,
	getStagedSource,
	displayStaged,
	trySyncStagedFromPreview,
} from "./state.js";
import {
	LoadPackFromRevID,
	LoadMgpackFromFile,
	ListInstalledMgpacks,
	ExportMgpack,
	DeleteInstalledMgpack,
	MergeInstalledMgpacks,
} from "../wailsjs/go/main/App.js";
import { editInstalledPackInBrowse } from "./browse.js";
import { syncMegiddoFromBackend } from "./status.js";

export async function refreshInstalledPacks() {
	const listEl = getEl("installedPacksEl");
	if (!listEl) return;
	listEl.replaceChildren();

	let list = [];
	try {
		list = await ListInstalledMgpacks();
	} catch (e) {
		const li = document.createElement("li");
		li.className = "muted";
		li.textContent = String(e);
		listEl.append(li);
		return;
	}

	if (!list?.length) {
		const li = document.createElement("li");
		li.className = "muted";
		li.textContent = "none yet - import a .mgpack or wiki oldid";
		listEl.append(li);
		syncMegiddoFromBackend().catch(console.error);
		return;
	}

	for (const row of list) {
		const li = document.createElement("li");

		const check = document.createElement("input");
		check.type = "checkbox";
		check.checked = selectedProfileIDs.has(row.profile_id);
		check.onchange = async () => {
			if (check.checked) selectedProfileIDs.add(row.profile_id);
			else selectedProfileIDs.delete(row.profile_id);
			await stageSelectedProfiles();
			syncMegiddoFromBackend().catch(console.error);
		};

		const label = document.createElement("span");
		label.className = "pack-label";
		label.textContent = row.author
			? `${row.name || row.profile_id} - ${row.author}`
			: row.name || row.profile_id;
		label.title = row.profile_id;

		const edit = document.createElement("button");
		edit.type = "button";
		edit.className = "secondary ghost";
		edit.textContent = "edit";
		edit.onclick = () =>
			editInstalledPackInBrowse(row.profile_id).catch(console.error);

		const del = document.createElement("button");
		del.type = "button";
		del.className = "danger ghost";
		del.textContent = "delete";
		del.onclick = async () => {
			if (!confirm(`Delete "${row.profile_id}"?`)) return;
			await DeleteInstalledMgpack(row.profile_id);
			selectedProfileIDs.delete(row.profile_id);
			if (
				selectedProfileIDs.size === 0 &&
				getStagedSource().startsWith("profiles")
			) {
				clearStaged();
				displayStaged();
			}
			await refreshInstalledPacks();
		};

		li.append(check, label, edit, del);
		listEl.append(li);
	}
	syncMegiddoFromBackend().catch(console.error);
}

export async function stageSelectedProfiles() {
	const sel = Array.from(selectedProfileIDs);
	if (!sel.length) {
		if (getStagedSource().startsWith("profiles")) clearStaged();
		displayStaged();
		return;
	}
	const merged = await MergeInstalledMgpacks(sel);
	if (merged) {
		setStaged(merged, `profiles(${sel.length})`);
		displayStaged();
	}
}

export async function handleRevImport() {
	const n = Number(getEl("revId")?.value || 0);
	const p = await LoadPackFromRevID(n);
	if (p) {
		setStaged(p, "wiki oldid");
		displayStaged();
		await refreshInstalledPacks();
	}
}

export async function handleFileImport() {
	const p = await LoadMgpackFromFile();
	if (p) {
		setStaged(p, "mgpack");
		displayStaged();
		await refreshInstalledPacks();
	}
}

export async function handleExport() {
	trySyncStagedFromPreview();
	const staged = getStaged();
	if (!staged) throw new Error("nothing to export");
	const out = await ExportMgpack(staged);
	if (out) {
		setPackHint(`exported - ${out}`);
		(await import("../main.js")).activateTab("home");
		await refreshInstalledPacks();
	}
}
