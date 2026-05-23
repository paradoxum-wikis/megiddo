import { getEl, setPackHint } from "./dom.js";
import {
	browseDraftValues,
	selectedBrowseRole,
	selectedBrowseCharacter,
	selectedBrowseModel,
	setSelectedBrowseRole,
	setSelectedBrowseCharacter,
	setSelectedBrowseModel,
	populateMetaFromPack,
} from "./state.js";
import {
	rowCharacter,
	rowRole,
	rowModel,
	skinsForCharacter,
	pickDefaultSkin,
	coerceSlot,
	replacementMatchKey,
	parseReplaceWith,
	canClear,
	roleLabel,
} from "./utils.js";
import { PICKER_ROLES } from "./constants.js";
import { ensureCatalogue, activateTab } from "../main.js";

function rowHoverTitle(row) {
	const chunks = [];
	if (row.notes) chunks.push(String(row.notes).trim());
	const rp = row.rbxm_props;
	if (rp) {
		const bits = [];
		if (rp.dump_label) bits.push(`dump ${rp.dump_label}`);
		if (rp.instance_path) bits.push(`path ${rp.instance_path}`);
		if (rp.class_name) bits.push(rp.class_name);
		if (rp.prop) bits.push(rp.prop);
		if (bits.length) chunks.push(`rbxm: ${bits.join(" - ")}`);
	}
	return chunks.join("\n\n");
}

function createNavBtn(text, active, onClick) {
	const btn = document.createElement("button");
	btn.type = "button";
	btn.className = `browse-nav-btn ${active ? "active" : "secondary"}`;
	btn.textContent = text;
	btn.onclick = onClick;
	return btn;
}

export async function editInstalledPackInBrowse(profileID) {
	const { LoadInstalledMgpack } = await import("../wailsjs/go/main/App.js");
	const p = await LoadInstalledMgpack(profileID);
	if (!p) return;
	const snap = await ensureCatalogue();
	populateMetaFromPack(p);
	browseDraftValues.clear();

	const byKey = new Map();
	for (const rep of p.replacements || []) {
		const slot =
			rep.slot != null && Number.isFinite(rep.slot) ? rep.slot : -1;
		byKey.set(
			replacementMatchKey(rep.target_id, slot, rep.asset_type),
			rep,
		);
	}

	let navSet = false;
	(snap.replacements_catalogue || []).forEach((row, idx) => {
		const key = replacementMatchKey(
			row.target_id,
			coerceSlot(row),
			row.asset_type,
		);
		const rep = byKey.get(key);
		if (!rep) return;
		if (rep.replace_with_file) {
			browseDraftValues.set(idx, {
				kind: "file",
				value: String(rep.replace_with_file),
			});
		} else if (rep.replace_with >= 0) {
			browseDraftValues.set(idx, {
				kind: "id",
				value: String(rep.replace_with),
			});
		}
		if (!navSet) {
			const ch = rowCharacter(row);
			setSelectedBrowseRole(rowRole(row));
			setSelectedBrowseCharacter(ch);
			setSelectedBrowseModel(rowModel(row));
			navSet = true;
		}
	});

	if (!navSet && p.replacements?.length) {
		const ch = p.replacements[0].character || "(misc)";
		setSelectedBrowseRole(rowRole(p.replacements[0]));
		setSelectedBrowseCharacter(ch);
		setSelectedBrowseModel("Default");
	}

	activateTab("browse");
	syncBrowseNavigation(snap);
	renderBrowseRows(snap);
	setPackHint(`editing ${profileID} in create - stage to home when done`);
}

export async function refreshBrowseTable() {
	const snap = await ensureCatalogue();
	syncBrowseNavigation(snap);
	renderBrowseRows(snap);
}

export function syncBrowseNavigation(snap) {
	const rows = snap?.replacements_catalogue || [];
	const roles = PICKER_ROLES.filter((r) =>
		rows.some((row) => rowRole(row) === r),
	);
	if (!roles.includes(selectedBrowseRole))
		setSelectedBrowseRole(roles[0] || "");

	const roleBtns = getEl("browseRoleButtons");
	if (roleBtns) {
		roleBtns.replaceChildren();
		roles.forEach((role) => {
			roleBtns.appendChild(
				createNavBtn(
					roleLabel(role),
					role === selectedBrowseRole,
					() => {
						setSelectedBrowseRole(role);
						setSelectedBrowseCharacter("");
						setSelectedBrowseModel("");
						syncBrowseNavigation(snap);
						renderBrowseRows(snap);
					},
				),
			);
		});
	}

	const chars = [
		...new Set(
			rows
				.filter(
					(r) =>
						rowRole(r) === selectedBrowseRole,
				)
				.map(rowCharacter),
		),
	].sort((a, b) => a.localeCompare(b));

	if (!chars.includes(selectedBrowseCharacter)) {
		setSelectedBrowseCharacter(chars[0] || "");
		setSelectedBrowseModel("");
	}

	const skins = selectedBrowseCharacter
		? skinsForCharacter(rows, selectedBrowseCharacter)
		: [];
	if (
		!selectedBrowseModel ||
		(selectedBrowseCharacter && !skins.includes(selectedBrowseModel))
	) {
		setSelectedBrowseModel(
			selectedBrowseCharacter
				? pickDefaultSkin(rows, selectedBrowseCharacter)
				: "",
		);
	}

	const charBtns = getEl("browseCharacterButtons");
	if (charBtns) {
		charBtns.replaceChildren();
		chars.forEach((ch) => {
			charBtns.appendChild(
				createNavBtn(ch, ch === selectedBrowseCharacter, () => {
					setSelectedBrowseCharacter(ch);
					setSelectedBrowseModel(pickDefaultSkin(rows, ch));
					syncBrowseNavigation(snap);
					renderBrowseRows(snap);
				}),
			);
		});
	}

	const skinBtns = getEl("browseSkinButtons");
	if (skinBtns && skins.length) {
		skinBtns.replaceChildren();
		skins.forEach((skin) => {
			const btn = createNavBtn(skin, skin === selectedBrowseModel, () => {
				setSelectedBrowseModel(skin);
				syncBrowseNavigation(snap);
				renderBrowseRows(snap);
			});
			btn.classList.add("browse-skin-btn");
			skinBtns.appendChild(btn);
		});
	}

	const hint = getEl("browseNavHint");
	if (hint) {
		const parts = [roleLabel(selectedBrowseRole)];
		if (selectedBrowseCharacter) parts.push(selectedBrowseCharacter);
		if (selectedBrowseModel) parts.push(selectedBrowseModel);
		hint.textContent = parts.join(" -> ");
	}
}

export function renderBrowseRows(snap) {
	const body = getEl("browseBody");
	if (!body) return;
	body.replaceChildren();
	const rows = snap?.replacements_catalogue || [];
	if (!rows.length) {
		const tr = document.createElement("tr");
		const td = document.createElement("td");
		td.colSpan = 5;
		td.className = "empty";
		td.textContent = "no catalogue rows";
		tr.appendChild(td);
		body.appendChild(tr);
		return;
	}

	const filtered = rows
		.map((row, idx) => ({ row, idx }))
		.filter(
			({ row }) =>
				rowRole(row) === selectedBrowseRole &&
				rowCharacter(row) === selectedBrowseCharacter &&
				rowModel(row) === selectedBrowseModel,
		)
		.sort((a, b) =>
			String(a.row.label || "").localeCompare(
				String(b.row.label || ""),
				undefined,
				{ sensitivity: "base" },
			),
		);

	if (!filtered.length) {
		const tr = document.createElement("tr");
		const td = document.createElement("td");
		td.colSpan = 5;
		td.className = "empty";
		td.textContent = "no rows for this filter";
		tr.appendChild(td);
		body.appendChild(tr);
		return;
	}

	for (const { row, idx } of filtered) {
		const tr = document.createElement("tr");
		if (rowHoverTitle(row)) tr.title = rowHoverTitle(row);

		const slot = coerceSlot(row);
		[
			row.label || "-",
			row.asset_type || "-",
			slot != null ? slot : "-",
			row.target_id != null ? row.target_id : "-",
		].forEach((t) => {
			const td = document.createElement("td");
			td.textContent = t;
			tr.appendChild(td);
		});

		const tdRw = document.createElement("td");
		tdRw.className = "rw-cell";

		const inp = document.createElement("input");
		inp.type = "text";
		inp.inputMode = "numeric";
		inp.className = "rw";
		inp.placeholder = "id, 0, or file…";

		const pick = document.createElement("button");
		pick.type = "button";
		pick.className = "rw-pick secondary";
		pick.textContent = "file";

		const clear = document.createElement("button");
		clear.type = "button";
		clear.className = "rw-clear secondary";
		clear.textContent = "×";

		const draft = browseDraftValues.get(idx);
		if (draft) {
			inp.value = draft.value;
			inp.readOnly = draft.kind === "file";
			if (draft.kind === "file") inp.classList.add("rw-file");
		}

		inp.oninput = () => {
			const v = inp.value.trim();
			if (!v) browseDraftValues.delete(idx);
			else browseDraftValues.set(idx, { kind: "id", value: v });
			inp.classList.remove("rw-file");
		};

		pick.onclick = async () => {
			const { PickLocalAssetFile } =
				await import("../wailsjs/go/main/App.js");
			const path = await PickLocalAssetFile(row.asset_type || "");
			if (!path) return;
			browseDraftValues.set(idx, { kind: "file", value: path });
			inp.value = path;
			inp.readOnly = true;
			inp.classList.add("rw-file");
		};

		clear.onclick = () => {
			browseDraftValues.delete(idx);
			inp.value = "";
			inp.readOnly = false;
			inp.classList.remove("rw-file");
		};

		tdRw.append(inp, pick, clear);
		tr.appendChild(tdRw);
		body.appendChild(tr);
	}
}

export function buildDraftFromBrowseInputs(snap) {
	const list = snap?.replacements_catalogue || [];
	const replacements = [];
	list.forEach((row, idx) => {
		const draft = browseDraftValues.get(idx);
		const out = {
			label: row.label || "",
			character: row.character ?? null,
			role: row.role || "",
			asset_type: row.asset_type || "",
			slot: coerceSlot(row),
			target_id: Number(row.target_id),
		};
		if (draft?.kind === "file") {
			if (!draft.value.trim()) return;
			out.replace_with_file = draft.value.trim();
		} else {
			const id = parseReplaceWith(draft?.value || "");
			if (id === null) return;
			if (id === 0 && !canClear(row.asset_type)) return;
			out.replace_with = id;
		}
		replacements.push(out);
	});
	if (!replacements.length) throw new Error("set at least one replacement");

	const meta = (id) => (getEl(id)?.value || "").trim();
	return {
		format_version: 1,
		name: meta("metaName") || "Megiddo browse draft",
		author: meta("metaAuthor"),
		version: meta("metaVersion") || "1.0.0",
		description: meta("metaDescription"),
		replacements,
	};
}
