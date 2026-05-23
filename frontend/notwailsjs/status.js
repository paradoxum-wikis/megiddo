import { getEl } from "./dom.js";
import { getStaged, selectedProfileIDs } from "./state.js";
import {
	IsMegiddoEnabled,
	GetRobloxCertPatchStatus,
	GetProxyStatus,
	GetLogs,
} from "../wailsjs/go/main/App.js";

export function updateMegiddoControl(on) {
	const btn = getEl("btnMegiddo");
	const help = getEl("megiddoHelp");
	if (!btn || !help) return;

	const hasSel =
		selectedProfileIDs.size > 0 ||
		!!getStaged() ||
		!!getEl("preview")?.value.trim();
	btn.disabled = !on && !hasSel;

	if (on) {
		btn.textContent = "disable megiddo";
		btn.classList.add("is-on");
		help.textContent = "on - swaps active in-game";
	} else {
		btn.textContent = "enable megiddo";
		btn.classList.remove("is-on");
		help.textContent = selectedProfileIDs.size
			? `off - ${selectedProfileIDs.size} pack(s) selected`
			: getStaged()
				? "off - enable to apply staged pack"
				: "off - pick a pack first";
	}
}

export async function syncMegiddoFromBackend() {
	const on = await IsMegiddoEnabled();
	updateMegiddoControl(on);
}

export function applyCertPatchPill(cert) {
	const chip = getEl("chipPatch");
	if (!chip) return;
	if (!cert?.total) {
		chip.textContent = "roblox patch - no installs";
		chip.className = "pill muted";
		return;
	}
	const { patched = 0, unpatched = 0, total = 0 } = cert;
	chip.textContent = `roblox patch - ${patched}/${total}`;
	chip.className =
		unpatched === 0 ? "pill ok" : patched === 0 ? "pill bad" : "pill warn";
}

export async function refreshCertPatchPill() {
	applyCertPatchPill(await GetRobloxCertPatchStatus());
}

export async function refreshProxyChips() {
	const st = await GetProxyStatus();
	await syncMegiddoFromBackend();
	const elev = getEl("chipElevated");
	if (elev) {
		elev.textContent = `admin elevated - ${st?.elevated ? "yes" : "no"}`;
		elev.className = `pill ${st?.elevated ? "ok" : "bad"}`;
	}
	const life = getEl("chipLifecycle");
	if (life) {
		life.textContent = st?.lastLifecycle?.trim()
			? `proxy - ${String(st.lastLifecycle).slice(0, 80)}`
			: `proxy - ${st?.active ? "running" : "idle"}`;
		life.className = st?.lastLifecycle?.trim()
			? "pill warn"
			: st?.active
				? "pill ok"
				: "pill muted";
	}
}

export async function refreshLogsPanel() {
	const el = getEl("logFull");
	if (el) el.textContent = (await GetLogs())?.join("\n") || "";
}

export async function refreshLiveStatus() {
	await Promise.all([refreshProxyChips(), refreshLogsPanel()]);
}
