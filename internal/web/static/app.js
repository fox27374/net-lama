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

const sections = ["overview", "agents", "tests", "sites", "results", "admin"];

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
async function loadAgents() {
  await fetchAgents();
  const tbody = $("#agents-table tbody");
  tbody.innerHTML = "";
  $("#agents-empty").classList.toggle("hidden", agents.length > 0);
  for (const a of agents) {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><span class="badge ${a.connected ? "on" : "off"}">${a.connected ? "connected" : "offline"}</span></td>
      <td><strong>${esc(a.name)}</strong></td>
      <td>${esc(a.siteName)}</td>
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
  return "nearest server";
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

function openSiteTestsDialog(site) {
  assigningSite = site;
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
  $(sel).addEventListener("change", loadResults));
$("#btn-results-refresh").addEventListener("click", loadResults);

const fmt = (v, digits = 1) =>
  v === undefined || v === null ? "–" : Number(v).toFixed(digits);

function resultDetails(r) {
  const p = r.payload || {};
  if (r.error) return `<span class="error">${esc(r.error)}</span>`;
  if (p.speedtest) {
    const s = p.speedtest;
    return `↓ ${fmt(s.downloadMbps)} Mbps · ↑ ${fmt(s.uploadMbps)} Mbps · ${fmt(s.latencyMs, 0)} ms · ${esc(s.serverName || "")}`;
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

  const c = $("#results-container");
  if (!results.length) {
    c.innerHTML = '<p class="empty">No results in this time window.</p>';
    return;
  }

  const rows = results.map((r) => `
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
