"use strict";

const $ = (sel) => document.querySelector(sel);

let me = null;
let tenants = [];
let sites = [];
let tests = [];
let agents = [];
let editingTest = null;

// --- Theme ---
function applyTheme(theme) {
  document.documentElement.dataset.theme = theme;
  localStorage.setItem("netlama-theme", theme);
  $("#theme-toggle").textContent = theme === "dark" ? "☀" : "☾";
}
$("#theme-toggle").addEventListener("click", () => {
  applyTheme(document.documentElement.dataset.theme === "dark" ? "light" : "dark");
  // Chart colors are theme-stepped; re-render with the new palette
  if (chartState && currentSection() === "results") {
    renderChart(chartState.results, chartState.windowSec);
  }
});
$("#theme-toggle").textContent =
  document.documentElement.dataset.theme === "dark" ? "☀" : "☾";

// --- API helper ---
async function api(method, path, body) {
  const opts = { method, headers: {} };
  if (body !== undefined) {
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(path, opts);
  if (res.status === 401) { showLogin(); throw new Error("not logged in"); }
  if (res.status === 204) return null;
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

// tenantId for scoped requests: admins use the header selector
function tenantParam(prefix = "?") {
  if (!me.isAdmin) return "";
  const id = $("#tenant-context").value;
  return id ? `${prefix}tenantId=${id}` : "";
}

function esc(s) {
  return String(s ?? "").replace(/[&<>"']/g, (c) =>
    ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

function dialogError(sel, msg) {
  const el = $(sel);
  el.textContent = msg;
  el.classList.toggle("hidden", !msg);
}

document.querySelectorAll("dialog [data-close]").forEach((b) =>
  b.addEventListener("click", () => b.closest("dialog").close()));

// --- Views ---
function showLogin() {
  $("#login-view").classList.remove("hidden");
  $("#app-view").classList.add("hidden");
}

async function showApp() {
  $("#login-view").classList.add("hidden");
  $("#app-view").classList.remove("hidden");
  $("#whoami").textContent = me.username + (me.isAdmin ? " · admin" : "");
  $("#nav-admin").classList.toggle("hidden", !me.isAdmin);
  if (me.isAdmin) {
    tenants = await api("GET", "/api/v1/tenants");
    const sel = $("#tenant-context");
    sel.classList.remove("hidden");
    const prev = sel.value;
    sel.innerHTML = tenants.map((t) => `<option value="${t.id}">${esc(t.name)}</option>`).join("");
    if (prev && tenants.some((t) => t.id === prev)) sel.value = prev;
  }
  showSection("overview");
}

const sections = ["overview", "agents", "tests", "sites", "results", "wireless", "path", "alerts", "logs", "apikeys", "admin"];

function showSection(name) {
  for (const sec of sections) $("#section-" + sec).classList.add("hidden");
  $("#section-" + name).classList.remove("hidden");
  document.querySelectorAll("nav button").forEach((b) =>
    b.classList.toggle("active", b.dataset.nav === name));
  reloadSection(name);
}

function currentSection() {
  return sections.find((s) => !$("#section-" + s).classList.contains("hidden"));
}

function reloadSection(name) {
  if (name === "overview") loadOverview();
  if (name === "agents") loadAgents();
  if (name === "tests") loadTests();
  if (name === "sites") loadSites();
  if (name === "results") initResults();
  if (name === "wireless") loadWireless();
  if (name === "path") loadPath();
  if (name === "alerts") loadAlerts();
  if (name === "logs") loadLogs();
  if (name === "apikeys") loadApiKeys();
  if (name === "admin") loadAdmin();
}

document.querySelectorAll("nav button").forEach((b) =>
  b.addEventListener("click", () => showSection(b.dataset.nav)));

$("#tenant-context").addEventListener("change", () => reloadSection(currentSection()));

// --- Login ---
$("#login-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  try {
    me = await api("POST", "/api/v1/login", {
      username: $("#login-username").value,
      password: $("#login-password").value,
    });
    $("#login-error").classList.add("hidden");
    await showApp();
  } catch (err) {
    $("#login-error").textContent = err.message;
    $("#login-error").classList.remove("hidden");
  }
});

$("#logout").addEventListener("click", async () => {
  await api("POST", "/api/v1/logout");
  me = null;
  showLogin();
});

// --- Overview ---
const HEALTH_LABEL = { healthy: "Healthy", degraded: "Degraded", failing: "Failing", nodata: "No data" };

async function loadOverview() {
  const ov = await api("GET", "/api/v1/overview" + tenantParam());
  $("#ov-sites").textContent = ov.sites;
  $("#ov-agents").textContent = ov.agents;
  $("#ov-agents-sub").textContent =
    ov.agents ? `${ov.agentsConnected} connected` : "";
  $("#ov-tests").textContent = ov.tests;
  updateAlertBadge(ov.activeAlerts);

  const health = ov.testHealth || [];
  const counts = { healthy: 0, degraded: 0, failing: 0, nodata: 0 };
  for (const h of health) counts[h.status] = (counts[h.status] || 0) + 1;

  const summary = $("#ov-health");
  summary.textContent = "";
  if (!health.length) {
    summary.textContent = "–";
  } else {
    for (const st of ["healthy", "degraded", "failing"]) {
      const span = document.createElement("span");
      span.className = "hcount " + st;
      span.textContent = `${counts[st]}`;
      span.title = HEALTH_LABEL[st];
      summary.appendChild(span);
    }
  }

  const tbody = $("#ov-health-table tbody");
  tbody.innerHTML = "";
  $("#ov-empty").classList.toggle("hidden", health.length > 0);
  for (const h of health) {
    const tr = document.createElement("tr");
    const last = h.lastSeen ? new Date(h.lastSeen).toLocaleString() : "—";
    const reporting = h.status === "nodata"
      ? "—"
      : `${h.ok}/${h.checks} checks OK · ${h.agents} agent${h.agents === 1 ? "" : "s"}`;
    tr.innerHTML = `
      <td><span class="health ${h.status}">${HEALTH_LABEL[h.status]}</span></td>
      <td><strong>${esc(h.name)}</strong></td>
      <td><span class="chip type-${h.type}">${h.type}</span></td>
      <td class="muted">${reporting}</td>
      <td class="muted nowrap">${last}</td>`;
    tr.addEventListener("click", () => {
      pendingResultTest = h.testId;
      showSection("results");
    });
    tbody.appendChild(tr);
  }
}

// --- Data helpers ---
async function fetchSites() {
  sites = await api("GET", "/api/v1/sites" + tenantParam());
  return sites;
}
async function fetchTests() {
  tests = await api("GET", "/api/v1/tests" + tenantParam());
  return tests;
}
async function fetchAgents() {
  agents = await api("GET", "/api/v1/agents" + tenantParam());
  return agents;
}

function testName(id) {
  const t = tests.find((t) => t.id === id);
  return t ? t.name : id;
}

// --- Agents ---
function formatBytes(bytes) {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
}

function statsText(stats) {
  if (!stats) return "—";
  let text = "";
  if (stats.cpuPercent !== undefined && stats.cpuPercent > 0) {
    text += stats.cpuPercent.toFixed(1) + "%";
  } else if (stats.cpuPercent === undefined) {
    text = "—";
  } else {
    text = "0%";
  }

  // Check staleness (> 2 minutes)
  const statsTime = new Date(stats.time).getTime();
  const now = Date.now();
  const stalenessMs = now - statsTime;
  if (stalenessMs > 2 * 60 * 1000) {
    text += " (stale)";
  }

  return text;
}

function memoryText(stats) {
  if (!stats || stats.memTotalBytes === 0) return "—";
  const used = formatBytes(stats.memUsedBytes);
  const total = formatBytes(stats.memTotalBytes);

  // Check staleness (> 2 minutes)
  const statsTime = new Date(stats.time).getTime();
  const now = Date.now();
  const stalenessMs = now - statsTime;
  const stale = stalenessMs > 2 * 60 * 1000 ? " (stale)" : "";

  return `${used} / ${total}${stale}`;
}

function diskText(stats) {
  if (!stats || stats.diskTotalBytes === 0) return "—";
  const used = formatBytes(stats.diskUsedBytes);
  const total = formatBytes(stats.diskTotalBytes);

  // Check staleness (> 2 minutes)
  const statsTime = new Date(stats.time).getTime();
  const now = Date.now();
  const stalenessMs = now - statsTime;
  const stale = stalenessMs > 2 * 60 * 1000 ? " (stale)" : "";

  return `${used} / ${total}${stale}`;
}

async function loadAgents() {
  await fetchAgents();
  const tbody = $("#agents-table tbody");
  tbody.innerHTML = "";
  $("#agents-empty").classList.toggle("hidden", agents.length > 0);
  for (const a of agents) {
    // Capability badges; an empty/missing list (old agent that never
    // reported capabilities) renders nothing.
    const caps = (a.capabilities || [])
      .map((c) => `<span class="chip type-${esc(c)}">${esc(c)}</span>`).join(" ");
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><span class="badge ${a.connected ? "on" : "off"}">${a.connected ? "connected" : "offline"}</span></td>
      <td><strong>${esc(a.name)}</strong></td>
      <td>${esc(a.siteName)}</td>
      <td>${caps}</td>
      <td class="muted">${statsText(a.stats)}</td>
      <td class="muted">${memoryText(a.stats)}</td>
      <td class="muted">${diskText(a.stats)}</td>
      <td class="muted">${new Date(a.createdAt).toLocaleString()}</td>
      <td style="text-align:right">
        <button class="ghost" data-edit>Edit</button>
        <button class="danger" data-del>Delete</button>
      </td>`;
    tr.querySelector("[data-edit]").addEventListener("click", () => openEditAgentDialog(a));
    tr.querySelector("[data-del]").addEventListener("click", async () => {
      if (!confirm(`Delete agent "${a.name}"? Its token stops working immediately.`)) return;
      await api("DELETE", `/api/v1/agents/${a.id}`);
      loadAgents();
    });
    tbody.appendChild(tr);
  }
}

$("#btn-new-agent").addEventListener("click", async () => {
  await fetchSites();
  if (!sites.length) { alert("Create a site first (Sites page)."); return; }
  $("#na-name").value = "";
  dialogError("#na-error", "");
  $("#na-site").innerHTML = sites.map((s) => `<option value="${s.id}">${esc(s.name)}</option>`).join("");
  $("#dlg-new-agent").showModal();
});

$("#form-new-agent").addEventListener("submit", async (e) => {
  e.preventDefault();
  try {
    const agent = await api("POST", "/api/v1/agents", {
      name: $("#na-name").value.trim(),
      siteId: $("#na-site").value,
    });
    $("#dlg-new-agent").close();
    $("#token-value").textContent = agent.token;
    $("#token-cmd").textContent =
      `podman run -d --name netlama-agent \\\n` +
      `  --sysctl net.ipv4.ping_group_range="0 65535" \\\n` +
      `  -e NETLAMA_SERVER=<server>:50051 \\\n` +
      `  -e NETLAMA_TOKEN=${agent.token} \\\n` +
      `  netlama-agent`;
    $("#dlg-token").showModal();
    loadAgents();
  } catch (err) {
    dialogError("#na-error", err.message);
  }
});

let editingAgent = null;

async function openEditAgentDialog(agent) {
  editingAgent = agent;
  await fetchSites();
  $("#ea-title").textContent = `Edit "${agent.name}"`;
  $("#ea-name").value = agent.name;
  $("#ea-site").innerHTML = sites.map((s) =>
    `<option value="${s.id}" ${s.id === agent.siteId ? "selected" : ""}>${esc(s.name)}</option>`).join("");
  dialogError("#ea-error", "");
  $("#dlg-edit-agent").showModal();
}

$("#form-edit-agent").addEventListener("submit", async (e) => {
  e.preventDefault();
  try {
    await api("PUT", `/api/v1/agents/${editingAgent.id}`, {
      name: $("#ea-name").value.trim(),
      siteId: $("#ea-site").value,
    });
    $("#dlg-edit-agent").close();
    loadAgents();
  } catch (err) {
    dialogError("#ea-error", err.message);
  }
});

$("#btn-copy-token").addEventListener("click", () => {
  navigator.clipboard.writeText($("#token-value").textContent);
  $("#btn-copy-token").textContent = "Copied!";
  setTimeout(() => { $("#btn-copy-token").textContent = "Copy token"; }, 1500);
});

// --- Tests ---
function paramsSummary(t) {
  const p = t.params || {};
  if (t.type === "ping") return `${(p.targets || []).join(", ")} · ${p.count || 5}x`;
  if (t.type === "dns") return `${(p.queries || []).join(", ")} @ ${(p.servers || []).join(", ")}`;
  if (t.type === "http") return p.url || "";
  if (t.type === "tcp") return (p.targets || []).join(", ");
  if (t.type === "wlan_scan") return "nearby access points";
  if (t.type === "traceroute") {
    const proto = (p.protocol || "tcp").toUpperCase();
    const port = (p.protocol === "icmp") ? "" : `:${p.port || 443}`;
    return `${p.target || ""} · ${proto}${port}`;
  }
  if (t.type === "speedtest") return speedtestProviderLabel(p.provider);
  return "nearest server";
}

function speedtestProviderLabel(provider) {
  switch (provider) {
    case "ndt7": return "M-Lab NDT7";
    case "cloudflare": return "Cloudflare";
    default: return "Ookla speedtest.net";
  }
}

async function loadTests() {
  await fetchTests();
  const tbody = $("#tests-table tbody");
  tbody.innerHTML = "";
  $("#tests-empty").classList.toggle("hidden", tests.length > 0);
  for (const t of tests) {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><strong>${esc(t.name)}</strong></td>
      <td><span class="chip type-${t.type}">${t.type}</span></td>
      <td class="muted">${t.intervalSeconds}s</td>
      <td class="muted">${esc(paramsSummary(t))}</td>
      <td style="text-align:right">
        <button class="ghost" data-edit>Edit</button>
        <button class="danger" data-del>Delete</button>
      </td>`;
    tr.querySelector("[data-edit]").addEventListener("click", () => openTestDialog(t));
    tr.querySelector("[data-del]").addEventListener("click", async () => {
      if (!confirm(`Delete test "${t.name}"? It is removed from all sites.`)) return;
      await api("DELETE", `/api/v1/tests/${t.id}`);
      loadTests();
    });
    tbody.appendChild(tr);
  }
}

function updateTestParamFields() {
  const type = $("#t-type").value;
  $("#t-params-ping").classList.toggle("hidden", type !== "ping");
  $("#t-params-dns").classList.toggle("hidden", type !== "dns");
  $("#t-params-http").classList.toggle("hidden", type !== "http");
  $("#t-params-tcp").classList.toggle("hidden", type !== "tcp");
  $("#t-params-wlan").classList.toggle("hidden", type !== "wlan_scan");
  $("#t-params-traceroute").classList.toggle("hidden", type !== "traceroute");
  $("#t-params-speedtest").classList.toggle("hidden", type !== "speedtest");
}
$("#t-type").addEventListener("change", updateTestParamFields);

function openTestDialog(test) {
  editingTest = test || null;
  $("#test-dlg-title").textContent = test ? `Edit "${test.name}"` : "New test";
  $("#t-submit").textContent = test ? "Save" : "Create";
  $("#t-type").disabled = !!test;
  dialogError("#t-error", "");

  $("#t-name").value = test ? test.name : "";
  $("#t-type").value = test ? test.type : "ping";
  $("#t-interval").value = test ? test.intervalSeconds : 60;
  const p = (test && test.params) || {};
  $("#t-ping-count").value = p.count || 5;
  $("#t-ping-targets").value = (p.targets || []).join("\n");
  $("#t-dns-queries").value = (p.queries || []).join("\n");
  $("#t-dns-servers").value = (p.servers || []).join("\n");
  $("#t-http-url").value = p.url || "";
  $("#t-http-timeout").value = p.timeoutSeconds || 10;
  $("#t-http-skiptls").checked = !!p.skipTlsVerify;
  $("#t-tcp-timeout").value = (test && test.type === "tcp" && p.timeoutSeconds) || 5;
  $("#t-tcp-targets").value = (test && test.type === "tcp" ? (p.targets || []) : []).join("\n");
  $("#t-tr-target").value = (test && test.type === "traceroute" && p.target) || "";
  $("#t-tr-protocol").value = (test && test.type === "traceroute" && p.protocol) || "tcp";
  $("#t-tr-port").value = (test && test.type === "traceroute" && p.port) || 443;
  $("#t-tr-maxhops").value = (test && test.type === "traceroute" && p.maxHops) || 30;
  $("#t-tr-probes").value = (test && test.type === "traceroute" && p.probesPerHop) || 5;
  $("#t-st-provider").value = (test && test.type === "speedtest" && p.provider) || "ookla";
  updateTestParamFields();
  $("#dlg-test").showModal();
}

$("#btn-new-test").addEventListener("click", () => openTestDialog(null));

function lines(v) {
  return v.split("\n").map((s) => s.trim()).filter(Boolean);
}

$("#form-test").addEventListener("submit", async (e) => {
  e.preventDefault();
  const type = $("#t-type").value;
  let params = {};
  if (type === "ping") {
    params = { targets: lines($("#t-ping-targets").value), count: +$("#t-ping-count").value };
  } else if (type === "dns") {
    params = { queries: lines($("#t-dns-queries").value), servers: lines($("#t-dns-servers").value) };
  } else if (type === "http") {
    params = { url: $("#t-http-url").value.trim(), timeoutSeconds: +$("#t-http-timeout").value, skipTlsVerify: $("#t-http-skiptls").checked };
  } else if (type === "tcp") {
    params = { targets: lines($("#t-tcp-targets").value), timeoutSeconds: +$("#t-tcp-timeout").value };
  } else if (type === "traceroute") {
    params = {
      target: $("#t-tr-target").value.trim(),
      protocol: $("#t-tr-protocol").value,
      port: +$("#t-tr-port").value,
      maxHops: +$("#t-tr-maxhops").value,
      probesPerHop: +$("#t-tr-probes").value,
    };
  } else if (type === "speedtest") {
    params = { provider: $("#t-st-provider").value };
  }
  const body = {
    name: $("#t-name").value.trim(),
    type,
    intervalSeconds: +$("#t-interval").value,
    params,
  };
  try {
    if (editingTest) {
      await api("PUT", `/api/v1/tests/${editingTest.id}`, body);
    } else {
      const tid = tenantParam("");
      if (tid) body.tenantId = tid.split("=")[1];
      await api("POST", "/api/v1/tests", body);
    }
    $("#dlg-test").close();
    loadTests();
  } catch (err) {
    dialogError("#t-error", err.message);
  }
});

// --- Sites ---
async function loadSites() {
  await Promise.all([fetchSites(), fetchTests()]);
  const tbody = $("#sites-table tbody");
  tbody.innerHTML = "";
  $("#sites-empty").classList.toggle("hidden", sites.length > 0);
  for (const s of sites) {
    const chips = (s.testIds || []).map((id) => `<span class="chip">${esc(testName(id))}</span>`).join(" ")
      || '<span class="muted">none</span>';
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><strong>${esc(s.name)}</strong></td>
      <td class="muted">${s.agents}</td>
      <td>${chips}</td>
      <td style="text-align:right">
        <button class="ghost" data-assign>Assign tests</button>
        <button class="danger" data-del>Delete</button>
      </td>`;
    tr.querySelector("[data-assign]").addEventListener("click", () => openSiteTestsDialog(s));
    tr.querySelector("[data-del]").addEventListener("click", async () => {
      if (!confirm(`Delete site "${s.name}"?`)) return;
      try {
        await api("DELETE", `/api/v1/sites/${s.id}`);
        loadSites();
      } catch (err) {
        alert(err.message);
      }
    });
    tbody.appendChild(tr);
  }
}

$("#btn-new-site").addEventListener("click", () => {
  $("#ns-name").value = "";
  dialogError("#ns-error", "");
  $("#dlg-new-site").showModal();
});

$("#form-new-site").addEventListener("submit", async (e) => {
  e.preventDefault();
  const body = { name: $("#ns-name").value.trim() };
  const tid = tenantParam("");
  if (tid) body.tenantId = tid.split("=")[1];
  try {
    await api("POST", "/api/v1/sites", body);
    $("#dlg-new-site").close();
    loadSites();
  } catch (err) {
    dialogError("#ns-error", err.message);
  }
});

let assigningSite = null;

// capabilityWarnings lists, for the given site's agents, every selected
// test whose type is missing from an agent's non-empty capability list.
// Agents with an empty list (old versions that never reported
// capabilities) are treated as able to run everything — no warning.
function capabilityWarnings(site, selectedTestIds) {
  const siteAgents = agents.filter((a) => a.siteId === site.id);
  const warnings = [];
  for (const id of selectedTestIds) {
    const t = tests.find((t) => t.id === id);
    if (!t) continue;
    for (const a of siteAgents) {
      const caps = a.capabilities || [];
      if (caps.length && !caps.includes(t.type)) {
        warnings.push(`${t.name} won't run on ${a.name} (no ${t.type} capability)`);
      }
    }
  }
  return warnings;
}

async function openSiteTestsDialog(site) {
  assigningSite = site;
  await fetchAgents();
  $("#st-dlg-title").textContent = `Tests for "${site.name}"`;
  dialogError("#st-error", "");
  $("#st-hint").textContent = tests.length
    ? "Changes are pushed live to the connected agents of this site."
    : "No tests defined yet — create them on the Tests page first.";
  $("#st-checkboxes").innerHTML = tests.map((t) => `
    <label class="check">
      <input type="checkbox" value="${t.id}" ${site.testIds.includes(t.id) ? "checked" : ""}>
      <strong>${esc(t.name)}</strong>&nbsp;<span class="chip type-${t.type}">${t.type}</span>
      <span class="muted">· ${t.intervalSeconds}s · ${esc(paramsSummary(t))}</span>
    </label>`).join("");
  const updateWarnings = () => {
    const selected = [...$("#st-checkboxes").querySelectorAll("input:checked")].map((i) => i.value);
    $("#st-warnings").innerHTML = capabilityWarnings(site, selected)
      .map((w) => `<p class="cap-warning">${esc(w)}</p>`).join("");
  };
  // Assignment (not addEventListener) so reopening the dialog never
  // stacks duplicate handlers on the persistent container element.
  $("#st-checkboxes").onchange = updateWarnings;
  updateWarnings();
  $("#dlg-site-tests").showModal();
}

$("#form-site-tests").addEventListener("submit", async (e) => {
  e.preventDefault();
  const testIds = [...$("#st-checkboxes").querySelectorAll("input:checked")].map((i) => i.value);
  try {
    await api("PUT", `/api/v1/sites/${assigningSite.id}/tests`, { testIds });
    $("#dlg-site-tests").close();
    loadSites();
  } catch (err) {
    dialogError("#st-error", err.message);
  }
});

// --- Results ---
let pendingResultTest = null; // set when jumping in from the overview

async function initResults() {
  await Promise.all([fetchSites(), fetchTests(), fetchAgents()]);
  fillFilter("#flt-site", sites, "All sites");
  fillFilter("#flt-agent", agents, "All agents");
  fillFilter("#flt-test", tests, "All tests");
  if (pendingResultTest) {
    $("#flt-test").value = pendingResultTest;
    pendingResultTest = null;
  }
  updateRunNowBtn();
  loadResults();
}

function fillFilter(sel, items, allLabel) {
  const el = $(sel);
  const prev = el.value;
  el.innerHTML = `<option value="">${allLabel}</option>` +
    items.map((i) => `<option value="${i.id}">${esc(i.name)}</option>`).join("");
  if (prev && items.some((i) => i.id === prev)) el.value = prev;
}

["#flt-window", "#flt-site", "#flt-agent", "#flt-test"].forEach((sel) =>
  $(sel).addEventListener("change", () => { updateRunNowBtn(); loadResults(); }));
$("#btn-results-refresh").addEventListener("click", loadResults);
$("#btn-results-run").addEventListener("click", () => {
  runTestNow($("#flt-agent").value, $("#flt-test").value, $("#btn-results-run"), loadResults);
});

// "Run now" needs both a specific agent and a specific test selected.
function updateRunNowBtn() {
  $("#btn-results-run").disabled = !($("#flt-agent").value && $("#flt-test").value);
}

const fmt = (v, digits = 1) =>
  v === undefined || v === null ? "–" : Number(v).toFixed(digits);

function resultDetails(r) {
  const p = r.payload || {};
  if (r.error) return `<span class="error">${esc(r.error)}</span>`;
  if (p.speedtest) {
    const s = p.speedtest;
    const provider = speedtestProviderLabel(s.provider);
    return `↓ ${fmt(s.downloadMbps)} Mbps · ↑ ${fmt(s.uploadMbps)} Mbps · ${fmt(s.latencyMs, 0)} ms · ${esc(s.serverName || "")} · <span class="muted">${esc(provider)}</span>`;
  }
  if (p.ping) {
    const g = p.ping;
    return `${esc(g.target)} · avg ${fmt(g.avgRttMs)} ms (${fmt(g.minRttMs)}–${fmt(g.maxRttMs)}) · loss ${fmt(g.lossPercent, 0)}%`;
  }
  if (p.dns) {
    const d = p.dns;
    return `${esc(d.query)} @ ${esc(d.server)} · ${fmt(d.resolveTimeMs)} ms · ${d.success ? "✓" : '<span class="error">✗</span>'}`;
  }
  if (p.http) {
    const h = p.http;
    const cert = h.certExpiryDays >= 0 ? ` · cert ${fmt(h.certExpiryDays, 0)}d` : "";
    return `HTTP ${h.statusCode} · ${fmt(h.totalMs)} ms (ttfb ${fmt(h.ttfbMs)})${cert}`;
  }
  if (p.tcp) {
    const t = p.tcp;
    return t.connected
      ? `${esc(t.target)} · connect ${fmt(t.connectMs)} ms`
      : `${esc(t.target)} · <span class="error">refused</span>`;
  }
  if (p.wlanScan) {
    const n = (p.wlanScan.accessPoints || []).length;
    return `${n} access point${n === 1 ? "" : "s"} on ${esc(p.wlanScan.interface || "?")}`;
  }
  if (p.traceroute) {
    const t = p.traceroute;
    const hops = (t.hops || []).length;
    return t.reached
      ? `${esc(t.target)} · reached in ${hops} hops · ${fmt(t.rttMs)} ms`
      : `${esc(t.target)} · <span class="error">stalled at hop ${t.failureHop || "?"}</span> of ${hops}`;
  }
  return "";
}

async function loadResults() {
  const windowSec = +$("#flt-window").value;
  const params = new URLSearchParams();
  const tid = tenantParam("");
  if (tid) params.set("tenantId", tid.split("=")[1]);
  if ($("#flt-site").value) params.set("siteId", $("#flt-site").value);
  if ($("#flt-agent").value) params.set("agentId", $("#flt-agent").value);
  if ($("#flt-test").value) params.set("testId", $("#flt-test").value);
  params.set("since", new Date(Date.now() - windowSec * 1000).toISOString());
  params.set("limit", "2000");

  const results = await api("GET", "/api/v1/results?" + params.toString());
  renderChart(results, windowSec);

  resultsAll = results;
  resultsPage = 0;
  renderResultsPage();
}

let resultsAll = [];
let resultsPage = 0;
const RESULTS_PER_PAGE = 10;

function renderResultsPage() {
  const c = $("#results-container");
  const pager = $("#results-pager");
  if (!resultsAll.length) {
    c.innerHTML = '<p class="empty">No results in this time window.</p>';
    pager.innerHTML = "";
    return;
  }

  const pages = Math.ceil(resultsAll.length / RESULTS_PER_PAGE);
  if (resultsPage > pages - 1) resultsPage = pages - 1;
  const start = resultsPage * RESULTS_PER_PAGE;
  const slice = resultsAll.slice(start, start + RESULTS_PER_PAGE);

  const rows = slice.map((r) => `
    <tr>
      <td class="muted nowrap">${new Date(r.time).toLocaleString()}</td>
      <td>${esc(r.siteName)}</td>
      <td>${esc(r.agentName)}</td>
      <td><strong>${esc(r.testName || "–")}</strong></td>
      <td><span class="chip type-${r.testType}">${r.testType}</span></td>
      <td>${resultDetails(r)}</td>
    </tr>`).join("");

  c.innerHTML = `<table>
    <thead><tr><th>Time</th><th>Site</th><th>Agent</th><th>Test</th><th>Type</th><th>Details</th></tr></thead>
    <tbody>${rows}</tbody></table>`;

  pager.innerHTML = "";
  if (pages <= 1) return;
  const prev = document.createElement("button");
  prev.className = "ghost";
  prev.textContent = "‹ Prev";
  prev.disabled = resultsPage === 0;
  prev.addEventListener("click", () => { resultsPage--; renderResultsPage(); });
  const next = document.createElement("button");
  next.className = "ghost";
  next.textContent = "Next ›";
  next.disabled = resultsPage >= pages - 1;
  next.addEventListener("click", () => { resultsPage++; renderResultsPage(); });
  const info = document.createElement("span");
  info.className = "muted";
  info.textContent = `Page ${resultsPage + 1} of ${pages} · ${resultsAll.length} results`;
  pager.append(prev, info, next);
}

// --- Timeline chart ---
// Line chart per dataviz method: 2px lines, hairline grid, crosshair +
// one tooltip for all series, legend for >=2 series, table view below.
let chartState = null; // {results, windowSec} for re-render on resize/theme

function seriesColor(i) {
  const style = getComputedStyle(document.documentElement);
  return style.getPropertyValue(`--series-${(i % 8) + 1}`).trim();
}

// Build series from results; only when a single test is selected does a
// timeline make sense (one metric, one unit).
function buildSeries(results) {
  const testId = $("#flt-test").value;
  if (!testId) return null;

  const asc = [...results].reverse().filter((r) => !r.error);
  if (!asc.length) return { series: [], unit: "" };

  const type = asc[0].testType;
  const groups = new Map();
  const add = (key, t, v) => {
    if (v === undefined || v === null) return;
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push({ t: new Date(t).getTime(), v });
  };

  let unit = "ms";
  for (const r of asc) {
    const p = r.payload || {};
    if (type === "ping" && p.ping) add(p.ping.target, r.time, p.ping.avgRttMs);
    else if (type === "dns" && p.dns) add(`${p.dns.query} @ ${p.dns.server}`, r.time, p.dns.resolveTimeMs);
    else if (type === "http" && p.http) {
      add("Total", r.time, p.http.totalMs);
      add("TTFB", r.time, p.http.ttfbMs);
    } else if (type === "tcp" && p.tcp) {
      if (p.tcp.connected) add(p.tcp.target, r.time, p.tcp.connectMs);
    } else if (type === "traceroute" && p.traceroute) {
      if (p.traceroute.reached) add("Path RTT", r.time, p.traceroute.rttMs);
    } else if (type === "speedtest" && p.speedtest) {
      unit = "Mbps";
      add("Download", r.time, p.speedtest.downloadMbps);
      add("Upload", r.time, p.speedtest.uploadMbps);
    }
  }

  const series = [...groups.keys()].sort().map((name, i) => ({
    name,
    color: seriesColor(i),
    points: groups.get(name),
  }));
  return { series, unit };
}

function niceMax(v) {
  if (v <= 0) return 1;
  const exp = Math.pow(10, Math.floor(Math.log10(v)));
  for (const m of [1, 2, 2.5, 5, 10]) {
    if (m * exp >= v) return m * exp;
  }
  return 10 * exp;
}

function fmtTick(ms, windowSec) {
  const d = new Date(ms);
  if (windowSec > 86400) {
    return d.toLocaleDateString(undefined, { month: "numeric", day: "numeric" }) +
      " " + d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  }
  return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
}

function renderChart(results, windowSec) {
  chartState = { results, windowSec };
  const area = $("#chart-area");
  area.innerHTML = "";

  const built = buildSeries(results);
  if (!built) {
    const hint = document.createElement("p");
    hint.className = "chart-hint";
    hint.textContent = "Select a test to see its timeline.";
    area.appendChild(hint);
    return;
  }
  if (!built.series.length) return;

  const { series, unit } = built;
  const NS = "http://www.w3.org/2000/svg";
  const W = Math.max(area.clientWidth || 600, 320);
  const H = 240;
  const M = { l: 48, r: 16, t: 12, b: 26 };
  const now = Date.now();
  const t0 = now - windowSec * 1000;
  const maxV = niceMax(Math.max(...series.flatMap((s) => s.points.map((p) => p.v))) * 1.05);
  const x = (t) => M.l + ((t - t0) / (now - t0)) * (W - M.l - M.r);
  const y = (v) => M.t + (1 - v / maxV) * (H - M.t - M.b);

  // Legend (only for >= 2 series)
  if (series.length >= 2) {
    const legend = document.createElement("div");
    legend.className = "chart-legend";
    for (const s of series) {
      const item = document.createElement("span");
      const key = document.createElement("span");
      key.className = "key";
      key.style.borderTopColor = s.color;
      item.appendChild(key);
      item.appendChild(document.createTextNode(s.name));
      legend.appendChild(item);
    }
    area.appendChild(legend);
  }

  const svg = document.createElementNS(NS, "svg");
  svg.setAttribute("viewBox", `0 0 ${W} ${H}`);
  svg.setAttribute("class", "chart-svg");
  svg.setAttribute("height", H);

  const el = (name, attrs, parent = svg) => {
    const node = document.createElementNS(NS, name);
    for (const [k, v] of Object.entries(attrs)) node.setAttribute(k, v);
    parent.appendChild(node);
    return node;
  };

  // Grid + y ticks (4 steps, clean numbers)
  for (let i = 0; i <= 4; i++) {
    const v = (maxV / 4) * i;
    el("line", { class: "grid", x1: M.l, x2: W - M.r, y1: y(v), y2: y(v) });
    const label = el("text", { x: M.l - 6, y: y(v) + 4, "text-anchor": "end" });
    label.textContent = v >= 100 ? Math.round(v).toLocaleString() : +v.toFixed(1);
  }
  // Unit label
  const unitLabel = el("text", { x: M.l - 6, y: M.t - 2, "text-anchor": "end" });
  unitLabel.textContent = unit;

  // X ticks
  for (let i = 0; i <= 4; i++) {
    const t = t0 + ((now - t0) / 4) * i;
    const anchor = i === 0 ? "start" : i === 4 ? "end" : "middle";
    const label = el("text", { x: x(t), y: H - 8, "text-anchor": anchor });
    label.textContent = fmtTick(t, windowSec);
  }

  const surface = getComputedStyle(document.documentElement).getPropertyValue("--surface").trim();

  // Series lines + end markers
  for (const s of series) {
    const d = s.points.map((p, i) => `${i ? "L" : "M"}${x(p.t).toFixed(1)},${y(p.v).toFixed(1)}`).join("");
    el("path", {
      d, fill: "none", stroke: s.color, "stroke-width": 2,
      "stroke-linecap": "round", "stroke-linejoin": "round",
    });
    const last = s.points[s.points.length - 1];
    el("circle", { cx: x(last.t), cy: y(last.v), r: 4, fill: s.color, stroke: surface, "stroke-width": 2 });
  }

  // Crosshair + tooltip
  const crosshair = el("line", { class: "crosshair", y1: M.t, y2: H - M.b, visibility: "hidden" });
  const tip = document.createElement("div");
  tip.className = "chart-tip hidden";
  area.appendChild(tip);

  const hover = el("rect", {
    x: M.l, y: M.t, width: W - M.l - M.r, height: H - M.t - M.b,
    fill: "transparent",
  });

  hover.addEventListener("pointermove", (ev) => {
    const rect = svg.getBoundingClientRect();
    const px = ((ev.clientX - rect.left) / rect.width) * W;
    const tAt = t0 + ((px - M.l) / (W - M.l - M.r)) * (now - t0);

    // Nearest point per series
    const rows = [];
    let nearestT = null, nearestDist = Infinity;
    for (const s of series) {
      let best = null, bestDist = Infinity;
      for (const p of s.points) {
        const dist = Math.abs(p.t - tAt);
        if (dist < bestDist) { bestDist = dist; best = p; }
      }
      if (best) {
        rows.push({ s, p: best });
        if (bestDist < nearestDist) { nearestDist = bestDist; nearestT = best.t; }
      }
    }
    if (nearestT === null) return;

    crosshair.setAttribute("x1", x(nearestT));
    crosshair.setAttribute("x2", x(nearestT));
    crosshair.setAttribute("visibility", "visible");

    tip.textContent = "";
    const timeDiv = document.createElement("div");
    timeDiv.className = "tip-time";
    timeDiv.textContent = new Date(nearestT).toLocaleString();
    tip.appendChild(timeDiv);
    for (const { s, p } of rows) {
      const row = document.createElement("div");
      row.className = "tip-row";
      const key = document.createElement("span");
      key.className = "key";
      key.style.borderTopColor = s.color;
      const val = document.createElement("span");
      val.className = "val";
      val.textContent = `${fmt(p.v)} ${unit}`;
      const name = document.createElement("span");
      name.className = "name";
      name.textContent = s.name;
      row.append(key, val, name);
      tip.appendChild(row);
    }
    tip.classList.remove("hidden");

    const areaRect = area.getBoundingClientRect();
    const tipX = ev.clientX - areaRect.left + 14;
    const flip = tipX + tip.offsetWidth + 10 > areaRect.width;
    tip.style.left = (flip ? tipX - tip.offsetWidth - 28 : tipX) + "px";
    tip.style.top = (ev.clientY - areaRect.top - 10) + "px";
  });
  hover.addEventListener("pointerleave", () => {
    crosshair.setAttribute("visibility", "hidden");
    tip.classList.add("hidden");
  });

  area.appendChild(svg);
}

// Re-render the chart on resize and theme change
let resizeTimer = null;
window.addEventListener("resize", () => {
  clearTimeout(resizeTimer);
  resizeTimer = setTimeout(() => {
    if (chartState && currentSection() === "results") {
      renderChart(chartState.results, chartState.windowSec);
    }
  }, 150);
});

// --- Wireless ---
let wlAgents = [];

async function loadWireless() {
  wlAgents = await api("GET", "/api/v1/agents" + tenantParam());
  const sel = $("#wl-agent");
  const prev = sel.value;
  sel.innerHTML = wlAgents.map((a) => `<option value="${a.id}">${esc(a.name)} (${esc(a.siteName)})</option>`).join("");
  $("#wl-noagents").classList.toggle("hidden", wlAgents.length > 0);
  $("#wl-body").classList.toggle("hidden", wlAgents.length === 0);
  if (!wlAgents.length) return;
  if (prev && wlAgents.some((a) => a.id === prev)) sel.value = prev;
  renderWirelessAgent();
}

$("#wl-agent").addEventListener("change", renderWirelessAgent);
$("#wl-refresh").addEventListener("click", renderWirelessAgent);

function currentWlAgent() {
  const id = $("#wl-agent").value;
  return wlAgents.find((a) => a.id === id);
}

async function renderWirelessAgent() {
  const agent = currentWlAgent();
  if (!agent) return;

  const ifaces = agent.wirelessInterfaces || [];
  const isel = $("#wl-iface");
  if (!ifaces.length) {
    isel.innerHTML = '<option value="">— none reported —</option>';
    $("#wl-iface-hint").textContent =
      "This agent reported no wireless interfaces. It needs a wireless adapter and the iw tool (or run it with NETLAMA_WLAN_DEMO=1 to try the workflow).";
  } else {
    isel.innerHTML = ifaces.map((w) => {
      const mon = w.supportsMonitor ? " · monitor-capable" : "";
      const selected = w.name === agent.wlanInterface ? "selected" : "";
      return `<option value="${esc(w.name)}" ${selected}>${esc(w.name)} (${esc(w.phy)})${mon}</option>`;
    }).join("");
    $("#wl-iface-hint").textContent = "The interface used for WLAN scan tests on this agent.";
  }
  $("#wl-iface-msg").textContent = "";

  // Latest WLAN scan result for this agent
  const params = new URLSearchParams({ agentId: agent.id, type: "wlan_scan", limit: "1" });
  const tid = tenantParam("");
  if (tid) params.set("tenantId", tid.split("=")[1]);
  const results = await api("GET", "/api/v1/results?" + params.toString());
  renderScan(results[0]);
}

function renderScan(result) {
  const tbody = $("#wl-ap-table tbody");
  const ssidBox = $("#wl-ssids");
  tbody.innerHTML = "";
  ssidBox.innerHTML = "";

  const aps = (result && result.payload && result.payload.wlanScan &&
    result.payload.wlanScan.accessPoints) || [];
  $("#wl-empty").classList.toggle("hidden", aps.length > 0 || !!result);
  const demo = result && result.payload && result.payload.wlanScan && result.payload.wlanScan.demo;
  const meta = $("#wl-scan-meta");
  meta.textContent = "";
  if (result) {
    if (demo) {
      const badge = document.createElement("span");
      badge.className = "demo-badge";
      badge.textContent = "DEMO DATA";
      meta.appendChild(badge);
    }
    meta.appendChild(document.createTextNode(
      `${aps.length} APs · ${new Date(result.time).toLocaleString()}`));
  }
  if (result && result.error) {
    $("#wl-empty").textContent = "Last scan failed: " + result.error;
    $("#wl-empty").classList.remove("hidden");
  }
  if (!aps.length) return;

  // SSID summary (grouped, hidden networks folded together)
  const bySsid = new Map();
  for (const ap of aps) {
    const name = ap.ssid || "— hidden —";
    bySsid.set(name, (bySsid.get(name) || 0) + 1);
  }
  for (const [name, count] of [...bySsid.entries()].sort()) {
    const chip = document.createElement("span");
    chip.className = "chip";
    chip.textContent = count > 1 ? `${name} ×${count}` : name;
    ssidBox.appendChild(chip);
  }

  // AP table, strongest signal first
  const sorted = [...aps].sort((a, b) => (b.rssiDbm || -999) - (a.rssiDbm || -999));
  for (const ap of sorted) {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><strong>${esc(ap.ssid || "— hidden —")}</strong></td>
      <td class="muted mono">${esc(ap.bssid)}</td>
      <td>${esc(ap.band || "")}</td>
      <td class="num">${ap.channel || "–"}</td>
      <td class="num">${signalCell(ap.rssiDbm)}</td>
      <td>${esc(ap.security || "")}</td>`;
    tbody.appendChild(tr);
  }
}

// signalCell colours the RSSI by rough quality.
function signalCell(rssi) {
  if (rssi === undefined || rssi === null) return "–";
  let cls = "sig-weak";
  if (rssi >= -60) cls = "sig-good";
  else if (rssi >= -72) cls = "sig-ok";
  return `<span class="${cls}">${fmt(rssi, 0)}</span>`;
}

$("#wl-iface-save").addEventListener("click", async () => {
  const agent = currentWlAgent();
  if (!agent) return;
  try {
    await api("PUT", `/api/v1/agents/${agent.id}`, {
      name: agent.name,
      siteId: agent.siteId,
      wlanInterface: $("#wl-iface").value,
    });
    agent.wlanInterface = $("#wl-iface").value;
    $("#wl-iface-msg").textContent = "Saved — pushed to the agent if connected.";
  } catch (err) {
    $("#wl-iface-msg").textContent = "Error: " + err.message;
  }
});

// --- Path (traceroute) ---
let paAgents = [];
let paTests = [];

async function loadPath() {
  [paAgents, paTests] = await Promise.all([
    api("GET", "/api/v1/agents" + tenantParam()),
    api("GET", "/api/v1/tests" + tenantParam()),
  ]);
  paTests = paTests.filter((t) => t.type === "traceroute");

  const noTests = paTests.length === 0 || paAgents.length === 0;
  $("#pa-none").classList.toggle("hidden", !noTests);
  $("#pa-body").classList.toggle("hidden", noTests);
  if (noTests) return;

  const asel = $("#pa-agent");
  const aprev = asel.value;
  asel.innerHTML = paAgents.map((a) => `<option value="${a.id}">${esc(a.name)} (${esc(a.siteName)})</option>`).join("");
  if (aprev && paAgents.some((a) => a.id === aprev)) asel.value = aprev;

  const tsel = $("#pa-test");
  const tprev = tsel.value;
  tsel.innerHTML = paTests.map((t) => `<option value="${t.id}">${esc(t.name)}</option>`).join("");
  if (tprev && paTests.some((t) => t.id === tprev)) tsel.value = tprev;

  renderPath();
}

$("#pa-agent").addEventListener("change", renderPath);
$("#pa-test").addEventListener("change", renderPath);
$("#pa-refresh").addEventListener("click", renderPath);
$("#pa-run").addEventListener("click", async () => {
  const btn = $("#pa-run");
  await runTestNow($("#pa-agent").value, $("#pa-test").value, btn, renderPath);
});

// runTestNow triggers an on-demand run and refreshes after a short delay.
async function runTestNow(agentId, testId, btn, refresh) {
  if (!agentId || !testId) return;
  const label = btn.textContent;
  btn.disabled = true;
  btn.textContent = "Running…";
  try {
    await api("POST", `/api/v1/agents/${agentId}/run`, { testId });
    // Give the agent a few seconds to run and stream the result back.
    setTimeout(async () => {
      await refresh();
      btn.disabled = false;
      btn.textContent = label;
    }, 6000);
  } catch (err) {
    alert(err.message);
    btn.disabled = false;
    btn.textContent = label;
  }
}

async function renderPath() {
  const agentId = $("#pa-agent").value;
  const testId = $("#pa-test").value;
  if (!agentId || !testId) return;

  const params = new URLSearchParams({ agentId, testId, type: "traceroute", limit: "1" });
  const tid = tenantParam("");
  if (tid) params.set("tenantId", tid.split("=")[1]);
  const results = await api("GET", "/api/v1/results?" + params.toString());

  const statusEl = $("#pa-status");
  const chainEl = $("#pa-chain");
  const tbody = $("#pa-hop-table tbody");
  statusEl.innerHTML = "";
  chainEl.innerHTML = "";
  tbody.innerHTML = "";
  $("#pa-meta").textContent = "";

  if (!results.length) {
    statusEl.innerHTML = '<p class="empty">No path results yet for this agent + test.</p>';
    return;
  }
  const r = results[0];
  const t = r.payload && r.payload.traceroute;
  if (!t) {
    statusEl.innerHTML = '<p class="empty">No path data in the latest result.</p>';
    return;
  }

  // Status banner
  const agent = paAgents.find((a) => a.id === agentId);
  const demo = t.demo ? '<span class="demo-badge">DEMO DATA</span>' : "";
  if (r.error) {
    statusEl.innerHTML = `${demo}<span class="health failing">Error</span> ${esc(r.error)}`;
  } else if (t.reached) {
    statusEl.innerHTML = `${demo}<span class="health healthy">Reached ${esc(t.target)}</span>` +
      ` <span class="muted">${(t.hops || []).length} hops · ${fmt(t.rttMs)} ms round-trip</span>`;
  } else {
    statusEl.innerHTML = `${demo}<span class="health failing">Path stalled</span>` +
      ` <span class="muted">to ${esc(t.target)} — last response at hop ${t.failureHop || "?"}, then no reply</span>`;
  }
  $("#pa-meta").textContent = new Date(r.time).toLocaleString();

  // Hop chain: agent -> hops -> target
  chainEl.appendChild(chainNode("This agent", esc(agent ? agent.name : ""), "node-endpoint", ""));
  const hops = t.hops || [];
  for (const h of hops) {
    chainEl.appendChild(chainArrow());
    const anon = !h.host;
    const cls = anon ? "node-anon"
      : (h.ttl === t.failureHop && !t.reached) ? "node-fail"
      : lossClass(h.lossPercent);
    const host = anon ? "* * *" : h.host;
    const sub = anon ? "no reply" : `${fmt(h.avgRttMs)} ms · ${fmt(h.lossPercent, 0)}% loss`;
    chainEl.appendChild(chainNode(`Hop ${h.ttl}`, esc(host), cls, sub));
  }
  if (!t.reached) {
    chainEl.appendChild(chainArrow(true));
    chainEl.appendChild(chainNode("", esc(t.target), "node-unreached", "unreached"));
  }

  // Per-hop latency chart
  renderHopChart(hops, t);

  // Hop table
  for (const h of hops) {
    const tr = document.createElement("tr");
    const host = h.host ? `<span class="mono">${esc(h.host)}</span>` : '<span class="muted">* * *</span>';
    tr.innerHTML = `
      <td class="num">${h.ttl}</td>
      <td>${host}</td>
      <td class="num">${lossCell(h.lossPercent)}</td>
      <td class="num">${h.host ? fmt(h.avgRttMs) : "–"}</td>
      <td class="num">${h.host ? fmt(h.bestRttMs) : "–"}</td>
      <td class="num">${h.host ? fmt(h.worstRttMs) : "–"}</td>`;
    tbody.appendChild(tr);
  }
}

// renderHopChart draws a column per hop (avg RTT), coloured by loss, so
// the hop where latency or loss jumps is obvious across the whole path.
function renderHopChart(hops, t) {
  const area = $("#pa-latency");
  area.innerHTML = "";
  if (!hops.length) return;

  const NS = "http://www.w3.org/2000/svg";
  const W = Math.max(area.clientWidth || 600, 320);
  const H = 220;
  const M = { l: 44, r: 12, t: 12, b: 40 };
  const maxV = niceMax(Math.max(0.1, ...hops.map((h) => h.avgRttMs || 0)) * 1.1);
  const plotW = W - M.l - M.r;
  const plotH = H - M.t - M.b;
  const band = plotW / hops.length;
  const barW = Math.min(24, band - 6);
  const y = (v) => M.t + (1 - v / maxV) * plotH;

  const style = getComputedStyle(document.documentElement);
  const col = {
    ok: style.getPropertyValue("--series-1").trim(),
    warn: "#d98a00",
    bad: style.getPropertyValue("--bad").trim(),
    muted: style.getPropertyValue("--border").trim(),
    accent: style.getPropertyValue("--accent").trim(),
  };
  const surface = style.getPropertyValue("--surface").trim();

  const svg = document.createElementNS(NS, "svg");
  svg.setAttribute("viewBox", `0 0 ${W} ${H}`);
  svg.setAttribute("class", "chart-svg");
  svg.setAttribute("height", H);
  const el = (name, attrs, parent = svg) => {
    const n = document.createElementNS(NS, name);
    for (const [k, v] of Object.entries(attrs)) n.setAttribute(k, v);
    parent.appendChild(n);
    return n;
  };

  // Grid + y ticks (ms)
  for (let i = 0; i <= 4; i++) {
    const v = (maxV / 4) * i;
    el("line", { class: "grid", x1: M.l, x2: W - M.r, y1: y(v), y2: y(v) });
    const lab = el("text", { x: M.l - 6, y: y(v) + 4, "text-anchor": "end" });
    lab.textContent = v >= 100 ? Math.round(v) : +v.toFixed(0);
  }
  const unit = el("text", { x: M.l - 6, y: M.t - 2, "text-anchor": "end" });
  unit.textContent = "ms";

  const tip = document.createElement("div");
  tip.className = "chart-tip hidden";
  area.style.position = "relative";
  area.appendChild(tip);

  hops.forEach((h, i) => {
    const cx = M.l + band * i + band / 2;
    const anon = !h.host;
    const isTarget = !anon && (h.host === t.targetIp || h.host === t.target);
    let color = col.ok;
    if (h.lossPercent >= 60) color = col.bad;
    else if (h.lossPercent >= 20) color = col.warn;
    if (isTarget && t.reached) color = col.accent;

    // x label: hop number
    const lab = el("text", { x: cx, y: H - 24, "text-anchor": "middle" });
    lab.textContent = h.ttl;

    if (anon) {
      // no reply: a muted baseline tick with a star
      const star = el("text", { x: cx, y: y(0) - 4, "text-anchor": "middle", fill: col.muted });
      star.textContent = "✳";
      return;
    }

    const barH = Math.max(2, y(0) - y(h.avgRttMs));
    el("rect", {
      x: cx - barW / 2, y: y(h.avgRttMs), width: barW, height: barH,
      rx: 4, fill: color,
    });

    // Hover hit area (full band) + tooltip
    const hit = el("rect", { x: M.l + band * i, y: M.t, width: band, height: plotH, fill: "transparent" });
    hit.addEventListener("pointermove", (ev) => {
      tip.textContent = "";
      const head = document.createElement("div");
      head.className = "tip-time";
      head.textContent = `Hop ${h.ttl} · ${h.host}`;
      tip.appendChild(head);
      const rows = [
        ["avg", `${fmt(h.avgRttMs)} ms`],
        ["best / worst", `${fmt(h.bestRttMs)} / ${fmt(h.worstRttMs)} ms`],
        ["loss", `${fmt(h.lossPercent, 0)} %`],
      ];
      for (const [k, v] of rows) {
        const row = document.createElement("div");
        row.className = "tip-row";
        const val = document.createElement("span");
        val.className = "val";
        val.textContent = v;
        const name = document.createElement("span");
        name.className = "name";
        name.textContent = k;
        row.append(val, name);
        tip.appendChild(row);
      }
      tip.classList.remove("hidden");
      const rect = area.getBoundingClientRect();
      const tx = ev.clientX - rect.left + 14;
      const flip = tx + tip.offsetWidth + 10 > rect.width;
      tip.style.left = (flip ? tx - tip.offsetWidth - 28 : tx) + "px";
      tip.style.top = (ev.clientY - rect.top - 10) + "px";
    });
    hit.addEventListener("pointerleave", () => tip.classList.add("hidden"));
  });

  // x-axis caption
  const cap = el("text", { x: M.l + plotW / 2, y: H - 6, "text-anchor": "middle", fill: col.muted });
  cap.textContent = "hop";

  area.appendChild(svg);
}

function chainNode(label, host, cls, sub) {
  const node = document.createElement("div");
  node.className = "hop-node " + cls;
  node.innerHTML = `<div class="hop-label">${label}</div>` +
    `<div class="hop-host">${host}</div>` +
    (sub ? `<div class="hop-sub">${sub}</div>` : "");
  return node;
}

function chainArrow(broken) {
  const a = document.createElement("div");
  a.className = "hop-arrow" + (broken ? " broken" : "");
  a.textContent = broken ? "✕" : "›";
  return a;
}

function lossClass(loss) {
  if (loss >= 60) return "node-bad";
  if (loss >= 20) return "node-warn";
  return "node-ok";
}

function lossCell(loss) {
  const cls = loss >= 60 ? "sig-weak" : loss >= 20 ? "sig-ok" : "sig-good";
  return `<span class="${cls}">${fmt(loss, 0)}%</span>`;
}

// --- Alerts ---
const METRIC_LABEL = {
  unhealthy: "unhealthy", latency_ms: "latency (ms)", loss_percent: "loss (%)",
  download_mbps: "download (Mbps)", upload_mbps: "upload (Mbps)",
};

function updateAlertBadge(count) {
  const badge = $("#nav-alert-badge");
  badge.textContent = count > 0 ? count : "";
  badge.classList.toggle("hidden", !count);
}

async function loadAlerts() {
  const [alerts, rules] = await Promise.all([
    api("GET", "/api/v1/alerts" + tenantParam()),
    api("GET", "/api/v1/alert-rules" + tenantParam()),
  ]);

  updateAlertBadge(alerts.filter((a) => a.state === "firing").length);

  const at = $("#alerts-table tbody");
  at.innerHTML = "";
  $("#alerts-empty").classList.toggle("hidden", alerts.length > 0);
  for (const a of alerts) {
    const firing = a.state === "firing";
    const since = firing ? new Date(a.startedAt).toLocaleString()
      : `${new Date(a.startedAt).toLocaleString()} → ${a.resolvedAt ? new Date(a.resolvedAt).toLocaleString() : ""}`;
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><span class="health ${firing ? "failing" : "healthy"}">${firing ? "Firing" : "Resolved"}</span></td>
      <td><strong>${esc(a.ruleName)}</strong></td>
      <td>${esc(a.agentName)}</td>
      <td class="muted">${esc(a.message)}</td>
      <td class="muted nowrap">${since}</td>`;
    at.appendChild(tr);
  }

  const rt = $("#rules-table tbody");
  rt.innerHTML = "";
  $("#rules-empty").classList.toggle("hidden", rules.length > 0);
  for (const r of rules) {
    const cond = r.metric === "unhealthy"
      ? "is unhealthy"
      : `${METRIC_LABEL[r.metric] || r.metric} ${r.operator} ${r.threshold}`;
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><strong>${esc(r.name)}</strong></td>
      <td>${esc(r.testName)}</td>
      <td class="muted">${esc(cond)}</td>
      <td class="muted">${r.forCount}×</td>
      <td class="muted">${r.webhookUrl ? "✓" : "—"}</td>
      <td style="text-align:right"><button class="danger" data-del>Delete</button></td>`;
    tr.querySelector("[data-del]").addEventListener("click", async () => {
      if (!confirm(`Delete rule "${r.name}"?`)) return;
      await api("DELETE", `/api/v1/alert-rules/${r.id}`);
      loadAlerts();
    });
    rt.appendChild(tr);
  }
}

$("#ar-metric").addEventListener("change", () => {
  $("#ar-threshold-wrap").classList.toggle("hidden", $("#ar-metric").value === "unhealthy");
});

$("#btn-new-rule").addEventListener("click", async () => {
  const tests = await api("GET", "/api/v1/tests" + tenantParam());
  if (!tests.length) { alert("Create a test first."); return; }
  $("#ar-name").value = "";
  $("#ar-test").innerHTML = tests.map((t) => `<option value="${t.id}">${esc(t.name)} (${t.type})</option>`).join("");
  $("#ar-metric").value = "unhealthy";
  $("#ar-threshold-wrap").classList.add("hidden");
  $("#ar-threshold").value = 0;
  $("#ar-forcount").value = 2;
  $("#ar-webhook").value = "";
  dialogError("#ar-error", "");
  $("#dlg-rule").showModal();
});

$("#form-rule").addEventListener("submit", async (e) => {
  e.preventDefault();
  const metric = $("#ar-metric").value;
  const body = {
    name: $("#ar-name").value.trim(),
    testId: $("#ar-test").value,
    metric,
    operator: metric === "unhealthy" ? ">" : $("#ar-op").value,
    threshold: metric === "unhealthy" ? 0 : +$("#ar-threshold").value,
    forCount: +$("#ar-forcount").value,
    webhookUrl: $("#ar-webhook").value.trim(),
  };
  const tid = tenantParam("");
  if (tid) body.tenantId = tid.split("=")[1];
  try {
    await api("POST", "/api/v1/alert-rules", body);
    $("#dlg-rule").close();
    loadAlerts();
  } catch (err) {
    dialogError("#ar-error", err.message);
  }
});

// --- Logs ---
let logsAutoTimer = null;

async function loadLogs() {
  await fetchAgents();
  fillFilter("#lg-agent", agents, "All agents");
  $("#lg-source-wrap").classList.toggle("hidden", !me.isAdmin);
  await refreshLogs();
  scheduleLogsAutoRefresh();
}

// Auto-refresh every 5s while the Logs page is the visible section and
// the toggle is checked; harmless no-op otherwise (checked before fetching).
function scheduleLogsAutoRefresh() {
  clearInterval(logsAutoTimer);
  logsAutoTimer = setInterval(() => {
    if (currentSection() === "logs" && $("#lg-autorefresh").checked) refreshLogs();
  }, 5000);
}

async function refreshLogs() {
  const params = new URLSearchParams();
  const tid = tenantParam("");
  if (tid) params.set("tenantId", tid.split("=")[1]);
  if ($("#lg-agent").value) params.set("agentId", $("#lg-agent").value);
  if ($("#lg-level").value) params.set("level", $("#lg-level").value);
  if (me.isAdmin && $("#lg-source").value) params.set("source", $("#lg-source").value);
  params.set("limit", "500");

  const logs = await api("GET", "/api/v1/logs?" + params.toString());
  renderLogs(logs);
}

function renderLogs(logs) {
  const c = $("#logs-container");
  $("#logs-empty").classList.toggle("hidden", logs.length > 0);
  if (!logs.length) {
    c.innerHTML = "";
    return;
  }
  const rows = logs.map((l) => `
    <tr>
      <td class="muted nowrap">${new Date(l.time).toLocaleString()}</td>
      <td><span class="chip type-${l.source}">${esc(l.source)}</span></td>
      <td>${esc(l.source === "server" ? "–" : (l.agentName || "?"))}</td>
      <td><span class="log-level ${esc((l.level || "").toLowerCase())}">${esc(l.level)}</span></td>
      <td class="log-msg">${esc(l.message)}</td>
    </tr>`).join("");
  c.innerHTML = `<table>
    <thead><tr><th>Time</th><th>Source</th><th>Agent</th><th>Level</th><th>Message</th></tr></thead>
    <tbody>${rows}</tbody></table>`;
}

$("#lg-agent").addEventListener("change", refreshLogs);
$("#lg-level").addEventListener("change", refreshLogs);
$("#lg-source").addEventListener("change", refreshLogs);
$("#btn-logs-refresh").addEventListener("click", refreshLogs);

// --- API Keys ---
async function loadApiKeys() {
  const keys = await api("GET", "/api/v1/apikeys");
  const tbody = $("#apikeys-table tbody");
  tbody.innerHTML = "";
  $("#apikeys-empty").classList.toggle("hidden", keys.length > 0);
  for (const k of keys) {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><strong>${esc(k.name)}</strong></td>
      <td class="muted">${esc(k.prefix)}&hellip;</td>
      <td class="muted nowrap">${new Date(k.createdAt).toLocaleString()}</td>
      <td class="muted nowrap">${k.lastUsedAt ? new Date(k.lastUsedAt).toLocaleString() : "never"}</td>
      <td style="text-align:right"><button class="danger" data-del>Revoke</button></td>`;
    tr.querySelector("[data-del]").addEventListener("click", async () => {
      if (!confirm(`Revoke API key "${k.name}"? Anything using it stops working immediately.`)) return;
      await api("DELETE", `/api/v1/apikeys/${k.id}`);
      loadApiKeys();
    });
    tbody.appendChild(tr);
  }
}

$("#btn-new-apikey").addEventListener("click", () => {
  $("#ak-name").value = "";
  dialogError("#ak-error", "");
  $("#dlg-new-apikey").showModal();
});

$("#form-new-apikey").addEventListener("submit", async (e) => {
  e.preventDefault();
  try {
    const key = await api("POST", "/api/v1/apikeys", { name: $("#ak-name").value.trim() });
    $("#dlg-new-apikey").close();
    $("#apikey-value").textContent = key.secret;
    $("#apikey-cmd").textContent =
      `curl -H "Authorization: Bearer ${key.secret}" https://<server>/api/v1/me`;
    $("#dlg-apikey-created").showModal();
    loadApiKeys();
  } catch (err) {
    dialogError("#ak-error", err.message);
  }
});

$("#btn-copy-apikey").addEventListener("click", () => {
  navigator.clipboard.writeText($("#apikey-value").textContent);
  $("#btn-copy-apikey").textContent = "Copied!";
  setTimeout(() => { $("#btn-copy-apikey").textContent = "Copy key"; }, 1500);
});

// --- Admin ---
async function loadAdmin() {
  tenants = await api("GET", "/api/v1/tenants");
  const users = await api("GET", "/api/v1/users");

  const tt = $("#tenants-table tbody");
  tt.innerHTML = "";
  for (const t of tenants) {
    const tr = document.createElement("tr");
    tr.innerHTML = `<td><strong>${esc(t.name)}</strong></td><td class="muted">${t.id.slice(0, 8)}…</td>
      <td style="text-align:right"><button class="danger" data-del>Delete</button></td>`;
    tr.querySelector("[data-del]").addEventListener("click", async () => {
      if (!confirm(`Delete tenant "${t.name}" including all its sites, agents and users?`)) return;
      await api("DELETE", `/api/v1/tenants/${t.id}`);
      showApp();
    });
    tt.appendChild(tr);
  }

  const ut = $("#users-table tbody");
  ut.innerHTML = "";
  for (const u of users) {
    const tname = tenants.find((t) => t.id === u.tenantId)?.name || u.tenantId;
    const tr = document.createElement("tr");
    tr.innerHTML = `<td><strong>${esc(u.username)}</strong></td><td class="muted">${esc(u.isAdmin ? "—" : tname)}</td>
      <td>${u.isAdmin ? '<span class="badge on">admin</span>' : '<span class="badge off">user</span>'}</td>
      <td style="text-align:right">${u.id === me.id ? "" : '<button class="danger" data-del>Delete</button>'}</td>`;
    const del = tr.querySelector("[data-del]");
    if (del) del.addEventListener("click", async () => {
      if (!confirm(`Delete user "${u.username}"?`)) return;
      await api("DELETE", `/api/v1/users/${u.id}`);
      loadAdmin();
    });
    ut.appendChild(tr);
  }
}

$("#btn-new-tenant").addEventListener("click", () => {
  $("#nt-name").value = "";
  dialogError("#nt-error", "");
  $("#dlg-new-tenant").showModal();
});

$("#form-new-tenant").addEventListener("submit", async (e) => {
  e.preventDefault();
  try {
    await api("POST", "/api/v1/tenants", { name: $("#nt-name").value.trim() });
    $("#dlg-new-tenant").close();
    await showApp();
    showSection("admin");
  } catch (err) {
    dialogError("#nt-error", err.message);
  }
});

$("#btn-new-user").addEventListener("click", () => {
  $("#nu-name").value = "";
  $("#nu-password").value = "";
  $("#nu-admin").checked = false;
  dialogError("#nu-error", "");
  $("#nu-tenant").innerHTML = tenants.map((t) => `<option value="${t.id}">${esc(t.name)}</option>`).join("");
  $("#nu-tenant-wrap").classList.remove("hidden");
  $("#dlg-new-user").showModal();
});

$("#nu-admin").addEventListener("change", () =>
  $("#nu-tenant-wrap").classList.toggle("hidden", $("#nu-admin").checked));

$("#form-new-user").addEventListener("submit", async (e) => {
  e.preventDefault();
  const isAdmin = $("#nu-admin").checked;
  try {
    await api("POST", "/api/v1/users", {
      username: $("#nu-name").value.trim(),
      password: $("#nu-password").value,
      isAdmin,
      tenantId: isAdmin ? "" : $("#nu-tenant").value,
    });
    $("#dlg-new-user").close();
    loadAdmin();
  } catch (err) {
    dialogError("#nu-error", err.message);
  }
});

// --- Init ---
(async () => {
  try {
    me = await api("GET", "/api/v1/me");
    await showApp();
  } catch {
    /* not logged in: login view already shown by api() */
  }
})();
