// cpuids lookup — client-side only. Fetches data/cpu_models.json (a copy of
// the repo's data/cpu_models.json, placed here at deploy time — see
// .github/workflows/pages.yml) and resolves a pasted /proc/cpuinfo block
// against it. Mirrors the resolution rules in cpuids.go and
// linuxcpuinfo/linuxcpuinfo.go; keep the two in sync if either changes.

const REPO = "fcuny/cpuids";

const state = {
  x86: new Map(), // "vendor|family|model" -> row
  arm: new Map(), // "implementer|part" -> row
  manifest: null,
};

const els = {
  input: document.getElementById("input"),
  resolve: document.getElementById("resolve"),
  result: document.getElementById("result"),
  generatedAt: document.getElementById("generated-at"),
  schemaVersion: document.getElementById("schema-version"),
};

init();

async function init() {
  try {
    const resp = await fetch("data/cpu_models.json");
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    const file = await resp.json();
    for (const row of file.models) {
      if (row.arch === "x86_64" && row.family != null && row.model != null) {
        state.x86.set(x86Key(row.vendor, row.family, row.model), row);
      } else if (row.arch === "aarch64" && row.implementer_id != null && row.part_id != null) {
        state.arm.set(armKey(row.implementer_id, row.part_id), row);
      }
    }
    state.manifest = file.manifest;
    els.generatedAt.textContent = file.manifest.generated_at;
    els.schemaVersion.textContent = String(file.manifest.schema_version);
  } catch (err) {
    els.generatedAt.textContent = "unavailable";
    els.schemaVersion.textContent = "?";
    showError("Could not load the CPU database (" + err.message + "). Try reloading the page.");
  }
}

els.resolve.addEventListener("click", () => runResolve());

let debounce;
els.input.addEventListener("input", () => {
  clearTimeout(debounce);
  debounce = setTimeout(runResolve, 300);
});

function runResolve() {
  const text = els.input.value;
  if (!text.trim()) {
    els.result.hidden = true;
    return;
  }

  const fields = firstBlock(text);
  if (Object.keys(fields).length === 0) {
    showError(
      "Couldn't find a processor block in that text. Paste the full output of " +
        "`cat /proc/cpuinfo` (or at least its first block, up to the first blank line)."
    );
    return;
  }

  if ("cpu implementer" in fields) {
    resolveARM(fields);
  } else {
    resolveX86(fields);
  }
}

function resolveX86(fields) {
  const vendor = fields["vendor_id"];
  const famStr = fields["cpu family"];
  const modStr = fields["model"];
  if (!vendor || famStr === undefined || modStr === undefined) {
    showError(
      "That block doesn't have the x86 fields this needs: vendor_id, cpu family, model."
    );
    return;
  }
  const fam = parseInt(famStr, 10);
  const mod = parseInt(modStr, 10);
  if (!Number.isInteger(fam) || !Number.isInteger(mod)) {
    showError("cpu family / model didn't parse as integers.");
    return;
  }
  const row = state.x86.get(x86Key(vendor, fam, mod));
  const keyLabel = `vendor=${vendor} family=${fam} model=${mod}`;
  if (!row) {
    showNotFound(keyLabel, fields);
    return;
  }
  showModel(row, keyLabel);
}

function resolveARM(fields) {
  const implStr = fields["cpu implementer"];
  const partStr = fields["cpu part"];
  if (partStr === undefined) {
    showError("Found 'cpu implementer' but no 'cpu part' line.");
    return;
  }
  const impl = parseFlexibleUint(implStr);
  const part = parseFlexibleUint(partStr);
  if (!Number.isInteger(impl) || !Number.isInteger(part)) {
    showError("cpu implementer / cpu part didn't parse as integers.");
    return;
  }
  const row = state.arm.get(armKey(impl, part));
  const keyLabel = `implementer=0x${impl.toString(16)} part=0x${part.toString(16)}`;
  if (!row) {
    showNotFound(keyLabel, fields);
    return;
  }
  showModel(row, keyLabel);
}

function showModel(row, keyLabel) {
  const vendor = humanVendor(row.vendor, row.arch);
  const aliases = row.aliases && row.aliases.length ? row.aliases.join(", ") : "—";
  els.result.className = "result ok";
  els.result.hidden = false;
  els.result.innerHTML = `
    <h2>${escapeHTML(row.name)}</h2>
    <dl>
      <dt>Vendor</dt><dd>${escapeHTML(vendor)}</dd>
      <dt>Microarchitecture</dt><dd>${escapeHTML(row.microarch_codename || "—")}</dd>
      <dt>Segment</dt><dd>${escapeHTML(row.segment || "unknown")}</dd>
      <dt>Release year</dt><dd>${row.release_year || "—"}</dd>
      <dt>Aliases</dt><dd>${escapeHTML(aliases)}</dd>
    </dl>
    <div class="key">matched on ${escapeHTML(keyLabel)}</div>
  `;
}

function showNotFound(keyLabel, fields) {
  els.result.className = "result err";
  els.result.hidden = false;
  els.result.innerHTML = `
    <h2>Not in the database</h2>
    <p>Parsed <span class="key">${escapeHTML(keyLabel)}</span> but no matching row exists yet.</p>
    <label class="input-label" for="suggest-name">Know what this is? Suggest a name (optional):</label>
    <input type="text" id="suggest-name" class="suggest-name-input" placeholder="e.g. AMD EPYC 9005 (Turin)" />
    <button type="button" class="report-link" id="report-btn">Report this on GitHub →</button>
  `;
  document.getElementById("report-btn").addEventListener("click", () => {
    const suggestedName = document.getElementById("suggest-name").value.trim();
    window.open(buildIssueURL(keyLabel, fields, suggestedName), "_blank", "noopener");
  });
}

// buildIssueURL prefills the GitHub issue form at
// .github/ISSUE_TEMPLATE/unknown-cpu.yml — query param names must match that
// form's field ids.
function buildIssueURL(keyLabel, fields, suggestedName) {
  const params = new URLSearchParams();
  params.set("template", "unknown-cpu.yml");
  params.set("title", `Unknown CPU: ${keyLabel}`);
  params.set("raw", blockText(fields));
  if (suggestedName) params.set("suggested_name", suggestedName);
  return `https://github.com/${REPO}/issues/new?${params.toString()}`;
}

// blockText re-renders a parsed /proc/cpuinfo block back into "key : value"
// lines, the same shape a manual reporter would paste in.
function blockText(fields) {
  return Object.entries(fields)
    .map(([key, value]) => `${key}\t: ${value}`)
    .join("\n");
}

function showError(message) {
  els.result.className = "result err";
  els.result.hidden = false;
  els.result.innerHTML = `<p>${escapeHTML(message)}</p>`;
}

// firstBlock mirrors linuxcpuinfo.firstBlock: lines up to the first blank line,
// split on the first ':', keys lowercased and trimmed, values trimmed.
function firstBlock(text) {
  const out = {};
  const lines = text.split(/\r\n|\r|\n/);
  for (const line of lines) {
    if (line.trim() === "") {
      if (Object.keys(out).length > 0) break;
      continue;
    }
    const idx = line.indexOf(":");
    if (idx === -1) continue;
    const key = line.slice(0, idx).trim().toLowerCase();
    const value = line.slice(idx + 1).trim();
    out[key] = value;
  }
  return out;
}

// parseFlexibleUint mirrors linuxcpuinfo.parseUint: decimal, or 0x-prefixed hex.
function parseFlexibleUint(s) {
  s = s.trim();
  if (/^0x/i.test(s)) return parseInt(s.slice(2), 16);
  return parseInt(s, 10);
}

function x86Key(vendor, family, model) {
  return `${vendor}|${family}|${model}`;
}

function armKey(implementer, part) {
  return `${implementer}|${part}`;
}

// humanVendor mirrors cpuids.humanVendor.
function humanVendor(stored, arch) {
  if (arch === "x86_64") {
    switch (stored) {
      case "GenuineIntel": return "Intel";
      case "AuthenticAMD": return "AMD";
      case "HygonGenuine": return "Hygon";
      case "CentaurHauls": return "Centaur";
    }
  }
  return stored;
}

function escapeHTML(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  }[c]));
}
