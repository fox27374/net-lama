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
  $("#nav-admin-btn").classList.toggle("hidden", !me.isAdmin);
  if (me.isAdmin) {
    tenants = await api("GET", "/api/v1/tenants");
    const sel = $("#tenant-context");
    sel.classList.remove("hidden");
    const prev = sel.value;
    sel.innerHTML = tenants.map((t) => `<option value="${t.id}">${esc(t.name)}</option>`).join("");
    if (prev && tenants.some((t) => t.id === prev)) sel.value = prev;
  }
  showSection("dashboard");
}

const sections = ["dashboard", "agents", "tests", "sites", "results", "wireless", "path", "alerts", "logs", "apikeys", "admin"];

function showSection(name) {
  for (const sec of sections) $("#section-" + sec).classList.add("hidden");
  $("#section-" + name).classList.remove("hidden");
  document.querySelectorAll(".nav-item").forEach((b) => {
    b.classList.toggle("active", b.dataset.nav === name);
  });
  reloadSection(name);
}

function currentSection() {
  return sections.find((s) => !$("#section-" + s).classList.contains("hidden"));
}

function reloadSection(name) {
  if (name === "dashboard") loadDashboard();
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

// Navigation helper with optional filter presets
function navTo(section, presets = {}) {
  if (presets.testId !== undefined) pendingResultTest = presets.testId;
  if (presets.siteId !== undefined) pendingResultSite = presets.siteId;
  showSection(section);
}

document.querySelectorAll(".nav-item").forEach((b) => {
  b.addEventListener("click", () => {
    showSection(b.dataset.nav);
  });
});

// Stat tile navigation
document.querySelectorAll(".stat-tile").forEach((tile) => {
  tile.addEventListener("click", (e) => {
    e.preventDefault();
    navTo(tile.dataset.nav);
  });
});

// "View all" link navigation
document.querySelectorAll(".view-all-link").forEach((link) => {
  link.addEventListener("click", (e) => {
    e.preventDefault();
    navTo(link.dataset.nav);
  });
});

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

// --- Dashboard ---
const HEALTH_LABEL = { healthy: "Healthy", degraded: "Degraded", failing: "Failing", nodata: "No data" };

async function loadDashboard() {
  // Populate site filter
  await fetchSites();
  const siteFilter = $("#db-site-filter");
  const currentValue = siteFilter.value;
  siteFilter.innerHTML = '<option value="">All sites</option>' +
    sites.map((s) => `<option value="${s.id}">${esc(s.name)}</option>`).join("");
  if (currentValue) siteFilter.value = currentValue;

  // Load dashboard data with optional site filter
  const siteId = siteFilter.value;
  const param = tenantParam() + (siteId ? (tenantParam() ? "&" : "?") + `siteId=${siteId}` : "");
  const ov = await api("GET", "/api/v1/overview" + param);

  // Render stat tiles
  $("#db-sites").textContent = ov.sites;
  $("#db-agents").textContent = ov.agents;
  $("#db-agents-sub").textContent = ov.agents ? `${ov.agentsConnected} connected` : "";
  $("#db-tests").textContent = ov.tests;
  $("#db-alerts").textContent = ov.activeAlerts;
  updateAlertBadge(ov.activeAlerts);

  // Render sites table (filtered if needed)
  const sitesData = siteId ? sites.filter(s => s.id === siteId) : sites;
  renderDashboardSites(sitesData, ov.testHealth || []);

  // Render alerts table
  await renderDashboardAlerts(siteId);

  // Render tests table with sparklines
  renderDashboardTests(ov.testHealth || []);

  // Render wireless table
  await renderDashboardWireless(siteId);
}

// Set up site filter listener
$("#db-site-filter").addEventListener("change", () => {
  loadDashboard();
});

function renderDashboardSites(sitesData, testHealth) {
  const tbody = $("#db-sites-table tbody");
  tbody.innerHTML = "";
  $("#db-sites-empty").classList.toggle("hidden", sitesData.length > 0);

  for (const site of sitesData) {
    // Health rollup: statuses of the tests assigned to this site.
    const statuses = (testHealth || []).filter((h) => (site.testIds || []).includes(h.testId));
    const counts = {};
    for (const h of statuses) counts[h.status] = (counts[h.status] || 0) + 1;
    const chips = ["healthy", "degraded", "failing", "nodata"]
      .filter((st) => counts[st])
      .map((st) => `<span class="health ${st}">${counts[st]} ${HEALTH_LABEL[st].toLowerCase()}</span>`)
      .join(" ") || '<span class="muted">no tests</span>';
    const tr = document.createElement("tr");
    tr.classList.add("clickable-row");
    tr.tabIndex = 0;
    tr.innerHTML = `
      <td><strong>${esc(site.name)}</strong></td>
      <td>${site.agents}</td>
      <td>${chips}</td>`;
    tr.addEventListener("click", () => navTo("sites"));
    tr.addEventListener("keydown", (e) => {
      if (e.key === "Enter") navTo("sites");
    });
    tbody.appendChild(tr);
  }
}

async function renderDashboardAlerts(siteId) {
  const tbody = $("#db-alerts-table tbody");
  tbody.innerHTML = "";

  let alertsUrl = "/api/v1/alerts" + tenantParam();
  if (siteId) alertsUrl += (tenantParam() ? "&" : "?") + `siteId=${siteId}`;

  try {
    const alerts = await api("GET", alertsUrl);
    $("#db-alerts-empty").classList.toggle("hidden", alerts.length > 0);

    for (const a of alerts.slice(0, 5)) { // Show recent 5
      const tr = document.createElement("tr");
      tr.classList.add("clickable-row");
      tr.tabIndex = 0;
      const since = a.startedAt ? new Date(a.startedAt).toLocaleString() : "—";
      tr.innerHTML = `
        <td><span class="health ${a.state === 'firing' ? 'failing' : 'healthy'}">${a.state === 'firing' ? 'Firing' : 'Resolved'}</span></td>
        <td>${esc(a.ruleName)}</td>
        <td>${esc(a.agentName || '—')}</td>
        <td class="muted">${esc(a.message || a.subject)}</td>
        <td class="muted nowrap">${since}</td>`;
      tr.addEventListener("click", () => navTo("alerts"));
      tr.addEventListener("keydown", (e) => {
        if (e.key === "Enter") navTo("alerts");
      });
      tbody.appendChild(tr);
    }
  } catch (err) {
    console.error("Failed to load alerts:", err);
  }
}

function renderDashboardTests(health) {
  const tbody = $("#db-tests-table tbody");
  tbody.innerHTML = "";
  $("#db-tests-empty").classList.toggle("hidden", health.length > 0);

  for (const h of health) {
    const tr = document.createElement("tr");
    tr.classList.add("clickable-row");
    tr.tabIndex = 0;
    const reporting = h.status === "nodata"
      ? "—"
      : `${h.ok}/${h.checks}`;
    const sparkline = renderSparkline(h.series, h.unit);
    tr.innerHTML = `
      <td><span class="health ${h.status}">${HEALTH_LABEL[h.status]}</span></td>
      <td><strong>${esc(h.name)}</strong></td>
      <td><span class="chip type-${h.type}">${h.type}</span></td>
      <td class="muted">${reporting}</td>
      <td>${sparkline}</td>`;
    tr.addEventListener("click", () => {
      navTo("results", { testId: h.testId });
    });
    tr.addEventListener("keydown", (e) => {
      if (e.key === "Enter") navTo("results", { testId: h.testId });
    });
    tbody.appendChild(tr);
  }
}

function renderSparkline(series, unit) {
  if (!series || series.length === 0) {
    return '<span class="muted">—</span>';
  }

  // Create inline SVG sparkline: ~160x36px
  const width = 160, height = 36;
  const padding = 4;
  const innerWidth = width - 2 * padding;
  const innerHeight = height - 2 * padding;

  // Find min/max for scaling
  let min = Math.min(...series);
  let max = Math.max(...series);
  if (min === max) max = min + 1;
  const range = max - min;

  // Generate points
  let points = "";
  for (let i = 0; i < series.length; i++) {
    const x = padding + (i / (series.length - 1)) * innerWidth;
    const y = padding + innerHeight - ((series[i] - min) / range) * innerHeight;
    points += (i > 0 ? " " : "") + `${x.toFixed(2)},${y.toFixed(2)}`;
  }

  // Current value text (right-aligned)
  const current = series[series.length - 1];
  const currentText = current.toFixed(0) + (unit ? " " + unit : "");

  // SVG
  return `<svg width="160" height="36" viewBox="0 0 ${width} ${height}" style="vertical-align: middle; margin: 0 8px 0 0;">
    <polyline points="${points}" fill="none" stroke="var(--cat-1)" stroke-width="2" vector-effect="non-scaling-stroke"/>
    <circle cx="${padding + innerWidth}" cy="${padding + innerHeight - ((series[series.length - 1] - min) / range) * innerHeight}" r="2.5" fill="var(--cat-1)" opacity="0.6" vector-effect="non-scaling-stroke"/>
  </svg><span style="font-variant-numeric: tabular-nums; font-size: 0.85em; color: var(--muted-solid);">${esc(currentText)}</span>`;
}

async function renderDashboardWireless(siteId) {
  const tbody = $("#db-wireless-table tbody");
  tbody.innerHTML = "";

  // Fetch agents for the site (or all if no site filter)
  let agentsData = agents;
  if (siteId && agents.length) {
    agentsData = agents.filter(a => a.siteId === siteId);
  }

  try {
    // Fetch latest wireless scans
    let resultsUrl = "/api/v1/results?testType=wlan_scan&limit=100" + tenantParam();
    if (siteId) resultsUrl += (tenantParam() ? "&" : "?") + `siteId=${siteId}`;
    const results = await api("GET", resultsUrl);

    // Group by agent, keep latest per agent
    const latestByAgent = {};
    for (const r of results) {
      if (!latestByAgent[r.agentId]) {
        latestByAgent[r.agentId] = r;
      }
    }

    const aPs = Object.values(latestByAgent);
    $("#db-wireless-empty").classList.toggle("hidden", aPs.length > 0);

    // Parse payloads and render APs
    for (const result of aPs.slice(0, 20)) { // Show latest 20 APs
      try {
        const payload = typeof result.payload === "string" ? JSON.parse(result.payload) : result.payload;
        const aps = payload.APs || [];
        for (const ap of aps.slice(0, 3)) { // Show top 3 per agent
          const tr = document.createElement("tr");
          tr.classList.add("clickable-row");
          tr.tabIndex = 0;
          tr.innerHTML = `
            <td>${esc(result.agentName)}</td>
            <td>${esc(ap.SSID)}</td>
            <td class="mono">${esc(ap.BSSID)}</td>
            <td>${esc(ap.Band)}</td>
            <td class="num">${ap.Channel}</td>
            <td class="num sig-${ap.RSSI > -70 ? 'good' : (ap.RSSI > -80 ? 'ok' : 'weak')}">${ap.RSSI}</td>
            <td>${esc(ap.Security)}</td>`;
          tr.addEventListener("click", () => navTo("wireless"));
          tr.addEventListener("keydown", (e) => {
            if (e.key === "Enter") navTo("wireless");
          });
          tbody.appendChild(tr);
        }
      } catch (e) {
        // Skip malformed results
      }
    }
  } catch (err) {
    console.error("Failed to load wireless:", err);
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

// agentHealthBadge renders the agent self-health status with the firing
// reasons on hover and the agent process uptime; unknown (old agent or no
// self-metrics yet) renders muted.
const AGENT_HEALTH_CLASS = { healthy: "healthy", degraded: "degraded", unhealthy: "failing" };
function agentHealthBadge(h) {
  if (!h || !h.status || h.status === "unknown") return '<span class="health nodata">unknown</span>';
  const cls = AGENT_HEALTH_CLASS[h.status] || "nodata";
  const title = (h.reasons || []).join("; ");
  const up = h.uptimeSeconds ? ` <span class="muted">${humanUptime(h.uptimeSeconds)}</span>` : "";
  return `<span class="health ${cls}" title="${esc(title)}">${esc(h.status)}</span>${up}`;
}
function humanUptime(sec) {
  const d = Math.floor(sec / 86400), h = Math.floor((sec % 86400) / 3600), m = Math.floor((sec % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
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
      <td>${agentHealthBadge(a.health)}</td>
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
      `podman run -d --init --name netlama-agent \\\n` +
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
    const chips = (s.testIds || []).map((id) => {
      const t = tests.find((t) => t.id === id);
      return `<span class="chip${t ? ` type-${t.type}` : ""}">${esc(testName(id))}</span>`;
    }).join(" ")
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
let pendingResultSite = null; // set when jumping in from the overview or deep link

async function initResults() {
  await Promise.all([fetchSites(), fetchTests(), fetchAgents()]);
  fillFilter("#flt-site", sites, "All sites");
  fillFilter("#flt-agent", agents, "All agents");
  fillFilter("#flt-test", tests, "All tests");
  if (pendingResultTest) {
    $("#flt-test").value = pendingResultTest;
    pendingResultTest = null;
  }
  if (pendingResultSite) {
    $("#flt-site").value = pendingResultSite;
    pendingResultSite = null;
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
let paHistoryResults = null;  // cached for theme re-render
let paHeatmapInstance = null;  // ECharts instance
let paWaterfallInstance = null;  // ECharts waterfall instance
let paDisplayedResult = null;  // cached result for theme re-render and Back to latest
let paLatestResult = null;  // latest result for "Back to latest" button
let paMetric = "latency";  // toggle between "latency" and "loss"

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

// Metric toggle (Latency / Loss)
document.querySelectorAll(".seg-btn[data-metric]").forEach((btn) => {
  btn.addEventListener("click", () => {
    paMetric = btn.dataset.metric;

    // Update button states
    document.querySelectorAll(".seg-btn[data-metric]").forEach((b) => {
      b.classList.toggle("active", b.dataset.metric === paMetric);
    });

    // Update card titles
    const waterfallTitle = $("#pa-waterfall-title");
    const historyTitle = $("#pa-history-title");
    if (paMetric === "loss") {
      waterfallTitle.textContent = "Packet loss by hop";
      historyTitle.textContent = "Path history — loss";
    } else {
      waterfallTitle.textContent = "Latency contribution by hop";
      historyTitle.textContent = "Path history — avg RTT";
    }

    // Re-render both charts from cached data
    if (paDisplayedResult) {
      const t = paDisplayedResult.payload && paDisplayedResult.payload.traceroute;
      if (t) {
        renderPathWaterfall(t.hops || []);
      }
    }
    if (paHistoryResults) {
      renderPathHeatmap(paHistoryResults);
    }
  });
});

// Add theme toggle handler for path heatmap and waterfall
$("#theme-toggle").addEventListener("click", () => {
  if (currentSection() === "path") {
    if (paHistoryResults) {
      renderPathHeatmap(paHistoryResults);
    }
    if (paDisplayedResult) {
      const t = paDisplayedResult.payload && paDisplayedResult.payload.traceroute;
      if (t) {
        renderPathWaterfall(t.hops || []);
      }
    }
  }
});

// Handle window resize for heatmap
window.addEventListener("resize", () => {
  if (paHeatmapInstance && currentSection() === "path") {
    paHeatmapInstance.resize();
  }
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

  // Fetch latest result and history (last 48)
  const tid = tenantParam("");
  const params1 = new URLSearchParams({ agentId, testId, type: "traceroute", limit: "1" });
  const params2 = new URLSearchParams({ agentId, testId, type: "traceroute", limit: "48" });
  if (tid) {
    params1.set("tenantId", tid.split("=")[1]);
    params2.set("tenantId", tid.split("=")[1]);
  }
  const [results, historyResults] = await Promise.all([
    api("GET", "/api/v1/results?" + params1.toString()),
    api("GET", "/api/v1/results?" + params2.toString()),
  ]);

  paHistoryResults = historyResults;  // cache for theme re-render

  const statusEl = $("#pa-status");
  const subwayEl = $("#pa-subway");
  const tbody = $("#pa-hop-table tbody");
  statusEl.innerHTML = "";
  subwayEl.innerHTML = "";
  tbody.innerHTML = "";
  $("#pa-meta").textContent = "";

  if (!results.length) {
    statusEl.innerHTML = '<p class="empty">No path results yet for this agent + test.</p>';
    return;
  }

  const r = results[0];
  paLatestResult = r;  // cache for "Back to latest" button
  renderPathResult(r, paAgents.find((a) => a.id === agentId));

  // Attach click handler to heatmap (once at init)
  renderPathHeatmap(historyResults);
}

// Render one path result (status, subway, hops table, waterfall)
function renderPathResult(r, agent) {
  paDisplayedResult = r;  // cache for theme re-render

  const t = r.payload && r.payload.traceroute;
  const statusEl = $("#pa-status");
  const subwayEl = $("#pa-subway");
  const tbody = $("#pa-hop-table tbody");

  if (!t) {
    statusEl.innerHTML = '<p class="empty">No path data in this result.</p>';
    subwayEl.innerHTML = "";
    tbody.innerHTML = "";
    return;
  }

  // Status banner with "Viewing run from" indicator if not the latest
  const demo = t.demo ? '<span class="demo-badge">DEMO DATA</span>' : "";
  let statusContent = "";

  if (r !== paLatestResult) {
    const runTime = new Date(r.time).toLocaleString();
    statusContent = `<span class="viewing-indicator">Viewing run from ${runTime} — <button id="pa-back-latest" class="ghost">Back to latest</button></span>`;
    setTimeout(() => {
      const backBtn = $("#pa-back-latest");
      if (backBtn) {
        backBtn.addEventListener("click", () => {
          if (paLatestResult) {
            renderPathResult(paLatestResult, agent);
          }
        });
      }
    }, 0);
  }

  if (r.error) {
    statusEl.innerHTML = `${statusContent}${demo}<span class="health failing">Error</span> ${esc(r.error)}`;
  } else if (t.reached) {
    statusEl.innerHTML = `${statusContent}${demo}<span class="health healthy">Reached ${esc(t.target)}</span>` +
      ` <span class="muted">${(t.hops || []).length} hops · ${fmt(t.rttMs)} ms round-trip</span>`;
  } else {
    statusEl.innerHTML = `${statusContent}${demo}<span class="health failing">Path stalled</span>` +
      ` <span class="muted">to ${esc(t.target)} — last response at hop ${t.failureHop || "?"}, then no reply</span>`;
  }
  $("#pa-meta").textContent = new Date(r.time).toLocaleString();

  // Vertical subway line
  const hops = t.hops || [];
  renderPathSubway(agent, hops, t);

  // Hop table with latency range bars
  const maxWorst = Math.max(0.1, ...hops.filter((h) => h.host).map((h) => h.worstRttMs || 0));
  tbody.innerHTML = "";
  for (const h of hops) {
    const tr = document.createElement("tr");
    const host = h.host ? `<span class="mono">${esc(h.host)}</span>` : '<span class="muted">* * *</span>';
    const latencyCell = h.host ? renderLatencyBar(h.bestRttMs, h.avgRttMs, h.worstRttMs, maxWorst, h.lossPercent) : "–";
    tr.innerHTML = `
      <td class="num">${h.ttl}</td>
      <td>${host}</td>
      <td class="num">${lossCell(h.lossPercent)}</td>
      <td class="num">${h.host ? fmt(h.avgRttMs) : "–"}</td>
      <td class="num">${h.host ? fmt(h.bestRttMs) : "–"}</td>
      <td class="num">${h.host ? fmt(h.worstRttMs) : "–"}</td>
      <td>${latencyCell}</td>`;
    tbody.appendChild(tr);
  }

  // Waterfall chart
  renderPathWaterfall(hops);
}

// Render vertical subway line for path visualization
function renderPathSubway(agent, hops, t) {
  const container = $("#pa-subway");
  container.innerHTML = "";

  // Station: this agent
  const agentStation = document.createElement("div");
  agentStation.className = "path-station path-station-start";
  agentStation.innerHTML = `
    <div class="path-dot path-dot-endpoint"></div>
    <div class="path-label">This agent</div>
    <div class="path-host"><span class="mono">${esc(agent ? agent.name : "")}</span></div>`;
  container.appendChild(agentStation);

  // Hop stations
  for (let i = 0; i < hops.length; i++) {
    const h = hops[i];
    const anon = !h.host;
    const isFailed = h.ttl === t.failureHop && !t.reached;
    const dotClass = anon ? "path-dot-anon" : isFailed ? "path-dot-bad" : lossClass(h.lossPercent).replace("node-", "path-dot-");
    const railBroken = isFailed;

    const station = document.createElement("div");
    station.className = "path-station" + (railBroken ? " path-station-broken" : "");
    const host = anon ? "* * *" : h.host;
    const hostSpan = anon ? `<span class="muted">${host}</span>` : `<span class="mono">${esc(host)}</span>`;
    const sub = anon ? "no reply" : `${fmt(h.avgRttMs)} ms · ${fmt(h.lossPercent, 0)}% loss`;

    station.innerHTML = `
      <div class="path-dot ${dotClass}"></div>
      <div class="path-label">Hop ${h.ttl}</div>
      <div class="path-host">${hostSpan}</div>
      <div class="path-sub">${sub}</div>`;
    container.appendChild(station);
  }

  // Target station (unreached)
  if (!t.reached) {
    const targetStation = document.createElement("div");
    targetStation.className = "path-station path-station-unreached";
    targetStation.innerHTML = `
      <div class="path-dot path-dot-unreached"></div>
      <div class="path-label">Target</div>
      <div class="path-host"><span class="muted">${esc(t.target)}</span></div>
      <div class="path-sub">unreached</div>`;
    container.appendChild(targetStation);
  }
}

// Render waterfall chart showing latency contribution by hop (horizontal APM-style)
function renderPathWaterfall(hops) {
  const container = $("#pa-waterfall");

  // Count responding hops (those with a host)
  const respondingHops = hops.filter((h) => h.host);
  if (respondingHops.length < 2) {
    if (paWaterfallInstance) {
      paWaterfallInstance.dispose();
      paWaterfallInstance = null;
      container.style.height = "";
    }
    container.innerHTML = '<p class="empty">Need at least 2 responding hops to show per-hop breakdown.</p>';
    return;
  }

  const style = getComputedStyle(document.documentElement);
  const accentColor = style.getPropertyValue("--accent").trim();
  const badColor = style.getPropertyValue("--bad").trim();
  const okColor = style.getPropertyValue("--ok").trim();
  const warnColor = style.getPropertyValue("--warn").trim();
  const borderColor = style.getPropertyValue("--border").trim();
  const textColor = style.getPropertyValue("--fg").trim();
  const mutedColor = style.getPropertyValue("--muted-solid").trim();

  let option = {};

  if (paMetric === "latency") {
    // Calculate latency contributions (cumulative RTT deltas)
    const waterfallData = [];
    let prevCumulative = 0;
    let maxDelta = 0;
    let largestDeltaIndex = -1;

    for (let i = 0; i < hops.length; i++) {
      const h = hops[i];
      if (!h.host) continue;

      const currentCumulative = h.avgRttMs || 0;
      const delta = currentCumulative - prevCumulative;

      if (delta > maxDelta) {
        maxDelta = delta;
        largestDeltaIndex = waterfallData.length;
      }

      waterfallData.push({
        ttl: h.ttl,
        host: h.host,
        cumulative: currentCumulative,
        delta: delta,
        prevCumulative: prevCumulative,
      });

      prevCumulative = currentCumulative;
    }

    // Prepare data for horizontal stacked bars (waterfall effect)
    const baseData = [];
    const deltaData = [];

    for (let i = 0; i < waterfallData.length; i++) {
      const d = waterfallData[i];
      // Base: invisible bar to position the visible bar
      baseData.push(Math.min(d.prevCumulative, d.cumulative));
      // Delta: visible portion
      const barHeight = Math.abs(d.delta);
      deltaData.push(barHeight);
    }

    const maxCum = Math.max(...waterfallData.map((d) => d.cumulative));
    const maxNice = niceMax(maxCum * 1.1);

    // Truncate host label to ~24 chars
    const labels = waterfallData.map((d) => {
      const label = `${d.ttl}  ${d.host}`;
      return label.length > 24 ? label.substring(0, 21) + "…" : label;
    });

    option = {
      tooltip: {
        trigger: "axis",
        axisPointer: { type: "shadow" },
        formatter: function (params) {
          if (!params || !params[0]) return "";
          const seriesName = params[0].seriesName;
          if (seriesName === "base") return "";

          const dataIndex = params[0].dataIndex;
          const d = waterfallData[dataIndex];
          const sign = d.delta >= 0 ? "+" : "";
          const label = d.delta < 0 ? "faster than previous hop (jitter)" : `added latency`;
          return `<strong>${esc(d.host)}</strong> (TTL ${d.ttl})<br/>` +
            `${label}: ${sign}${fmt(d.delta)} ms<br/>` +
            `Cumulative: ${fmt(d.cumulative)} ms`;
        }
      },
      grid: { height: "70%", top: 10, bottom: 60, left: 140, right: 20 },
      xAxis: {
        type: "value",
        position: "top",
        min: 0,
        max: maxNice,
        name: "Latency (ms)",
        nameTextStyle: { color: mutedColor, fontSize: 11 },
        axisLabel: { color: mutedColor, fontSize: 11 },
        axisLine: { lineStyle: { color: mutedColor } },
        splitLine: { lineStyle: { color: mutedColor, opacity: 0.2 } },
      },
      yAxis: {
        type: "category",
        data: labels,
        inverse: true,
        axisLabel: { color: mutedColor, fontSize: 11, formatter: (val) => val.length > 24 ? val.substring(0, 21) + "…" : val },
        axisLine: { lineStyle: { color: mutedColor } },
      },
      series: [
        {
          name: "base",
          type: "bar",
          stack: "waterfall",
          data: baseData,
          itemStyle: { color: "transparent" },
          tooltip: { show: false },
        },
        {
          name: "Latency",
          type: "bar",
          stack: "waterfall",
          barWidth: 16,
          data: deltaData.map((val, i) => ({
            value: val,
            itemStyle: {
              color: i === largestDeltaIndex && waterfallData[i].delta > 0 ? badColor : waterfallData[i].delta < 0 ? borderColor : accentColor,
            }
          })),
          itemStyle: { borderWidth: 0 },
        }
      ],
      textStyle: { color: textColor },
    };
  } else {
    // Loss mode: plain horizontal bars, not cumulative
    const lossData = respondingHops.map((h) => ({
      ttl: h.ttl,
      host: h.host,
      loss: h.lossPercent || 0,
      avgRtt: h.avgRttMs || 0,
    }));

    // Truncate host label to ~24 chars
    const labels = lossData.map((d) => {
      const label = `${d.ttl}  ${d.host}`;
      return label.length > 24 ? label.substring(0, 21) + "…" : label;
    });

    const lossValues = lossData.map((d) => ({
      value: d.loss,
      itemStyle: {
        color: d.loss >= 60 ? badColor : d.loss >= 20 ? warnColor : okColor,
      }
    }));

    option = {
      tooltip: {
        trigger: "axis",
        axisPointer: { type: "shadow" },
        formatter: function (params) {
          if (!params || !params[0]) return "";
          const dataIndex = params[0].dataIndex;
          const d = lossData[dataIndex];
          return `<strong>${esc(d.host)}</strong> (TTL ${d.ttl})<br/>` +
            `Loss: ${fmt(d.loss, 0)}%<br/>` +
            `Avg RTT: ${fmt(d.avgRtt)} ms`;
        }
      },
      grid: { height: "70%", top: 10, bottom: 60, left: 140, right: 20 },
      xAxis: {
        type: "value",
        position: "top",
        min: 0,
        max: 100,
        name: "Loss (%)",
        nameTextStyle: { color: mutedColor, fontSize: 11 },
        axisLabel: { color: mutedColor, fontSize: 11 },
        axisLine: { lineStyle: { color: mutedColor } },
        splitLine: { lineStyle: { color: mutedColor, opacity: 0.2 } },
      },
      yAxis: {
        type: "category",
        data: labels,
        inverse: true,
        axisLabel: { color: mutedColor, fontSize: 11, formatter: (val) => val.length > 24 ? val.substring(0, 21) + "…" : val },
        axisLine: { lineStyle: { color: mutedColor } },
      },
      series: [
        {
          name: "Loss",
          type: "bar",
          barWidth: 16,
          data: lossValues,
          itemStyle: { borderWidth: 0 },
        }
      ],
      textStyle: { color: textColor },
    };
  }

  // Set chart height based on number of rows
  const chartHeight = Math.max(180, respondingHops.length * 28 + 70);

  // Init chart if needed
  if (!paWaterfallInstance) {
    container.innerHTML = "";
    container.style.height = chartHeight + "px";
    paWaterfallInstance = window.echarts.init(container);
    window.addEventListener("resize", () => {
      if (paWaterfallInstance && currentSection() === "path") {
        paWaterfallInstance.resize();
      }
    });
  } else {
    container.style.height = chartHeight + "px";
    paWaterfallInstance.resize();
  }

  paWaterfallInstance.setOption(option, true);
}

// Render latency range bar for a hop
function renderLatencyBar(best, avg, worst, maxWorst, loss) {
  if (!best || !avg || !worst) return "–";

  const bestPct = (best / maxWorst) * 100;
  const worstPct = (worst / maxWorst) * 100;
  const avgPct = (avg / maxWorst) * 100;
  const barColor = loss >= 60 ? "var(--bad)" : loss >= 20 ? "var(--warn)" : "var(--ok)";

  return `<div class="latency-bar-container">
    <div class="latency-track">
      <div class="latency-range" style="left: ${bestPct}%; width: ${worstPct - bestPct}%; background: ${barColor};"></div>
      <div class="latency-avg" style="left: ${avgPct}%;"></div>
    </div>
  </div>`;
}

// Render path history heatmap using ECharts (supports latency and loss modes)
async function renderPathHeatmap(results) {
  const container = $("#pa-history");

  if (!results || results.length < 2) {
    if (paHeatmapInstance) {
      paHeatmapInstance.dispose();
      paHeatmapInstance = null;
      container.style.height = "";
    }
    container.innerHTML = '<p class="empty">Need at least 2 results to show history.</p>';
    return;
  }

  // Reverse results so newest is on the right
  const reversed = [...results].reverse();

  // Collect all hops across all results; use r.time as category key
  const allHops = new Set();
  const heatmapData = [];
  let maxAvgRtt = 0;

  for (const r of reversed) {
    const t = r.payload && r.payload.traceroute;
    if (!t || !t.hops) continue;

    for (const h of t.hops) {
      if (h.host) {
        allHops.add(h.ttl);
        const value = paMetric === "loss" ? h.lossPercent : h.avgRttMs;
        heatmapData.push([
          r.time,  // raw r.time as category key
          h.ttl,
          value,
        ]);
        if (paMetric === "latency") {
          maxAvgRtt = Math.max(maxAvgRtt, h.avgRttMs || 0);
        }
      }
    }
  }

  const maxTtl = Math.max(...Array.from(allHops));
  const sortedHops = Array.from(allHops).sort((a, b) => a - b).reverse();  // inverted

  const style = getComputedStyle(document.documentElement);
  const okColor = style.getPropertyValue("--ok").trim();
  const warnColor = style.getPropertyValue("--warn").trim();
  const badColor = style.getPropertyValue("--bad").trim();
  const textColor = style.getPropertyValue("--fg").trim();
  const mutedColor = style.getPropertyValue("--muted-solid").trim();

  let visualMapConfig = {};
  if (paMetric === "loss") {
    visualMapConfig = {
      min: 0,
      max: 100,
      orient: "horizontal",
      bottom: 10,
      left: "center",
      inRange: { color: [okColor, warnColor, badColor] },
      textStyle: { color: mutedColor, fontSize: 11 },
    };
  } else {
    const maxNice = niceMax(maxAvgRtt * 1.1);
    visualMapConfig = {
      min: 0,
      max: maxNice,
      orient: "horizontal",
      bottom: 10,
      left: "center",
      inRange: { color: [okColor, "#ffd000", badColor] },
      textStyle: { color: mutedColor, fontSize: 11 },
    };
  }

  const option = {
    tooltip: {
      position: "top",
      formatter: function (params) {
        if (!params.data) return "";
        const timeStr = params.data[0];
        const ttl = params.data[1];
        // Find the original result using r.time
        const result = reversed.find((r) => r.time === timeStr);
        let hopInfo = "";
        if (result) {
          const t = result.payload.traceroute;
          const hop = (t.hops || []).find((h) => h.ttl === ttl);
          if (hop && hop.host) {
            if (paMetric === "loss") {
              hopInfo = `<br/>Host: ${hop.host}<br/>Loss: ${fmt(hop.lossPercent, 0)}%<br/>Avg RTT: ${fmt(hop.avgRttMs)} ms`;
            } else {
              hopInfo = `<br/>Host: ${hop.host}<br/>Best: ${fmt(hop.bestRttMs)} ms<br/>Worst: ${fmt(hop.worstRttMs)} ms<br/>Loss: ${fmt(hop.lossPercent, 0)}%`;
            }
          }
        }
        const displayTime = new Date(timeStr).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
        return `Time: ${displayTime}<br/>Hop: ${ttl}${hopInfo}`;
      }
    },
    grid: { height: "70%", top: 40, bottom: 60, left: 40, right: 20 },
    xAxis: {
      type: "category",
      data: [...new Set(heatmapData.map((d) => d[0]))],
      splitArea: { show: true },
      axisLabel: {
        color: mutedColor,
        fontSize: 11,
        formatter: function (value) {
          return new Date(value).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
        }
      },
      axisLine: { lineStyle: { color: mutedColor } },
    },
    yAxis: {
      type: "category",
      data: sortedHops,
      splitArea: { show: true },
      axisLabel: { color: mutedColor, fontSize: 11 },
      axisLine: { lineStyle: { color: mutedColor } },
    },
    visualMap: visualMapConfig,
    series: [
      {
        name: paMetric === "loss" ? "Loss (%)" : "Avg RTT (ms)",
        type: "heatmap",
        data: heatmapData,
        itemStyle: { borderWidth: 0 },
        emphasis: { itemStyle: { borderColor: textColor, borderWidth: 1 } },
      }
    ],
    textStyle: { color: textColor },
  };

  // Init chart if needed
  if (!paHeatmapInstance) {
    container.innerHTML = "";
    container.style.height = "400px";
    paHeatmapInstance = window.echarts.init(container);
    window.addEventListener("resize", () => {
      if (paHeatmapInstance && currentSection() === "path") {
        paHeatmapInstance.resize();
      }
    });

    // Attach click handler (only once at init)
    paHeatmapInstance.on("click", function (params) {
      if (!params.data || !params.data[0]) return;
      const timeStr = params.data[0];  // raw r.time from x-axis
      // Find result in cached history
      const result = paHistoryResults.find((r) => r.time === timeStr);
      if (result) {
        const agent = paAgents.find((a) => a.id === $("#pa-agent").value);
        renderPathResult(result, agent);
      }
    });
  }

  paHeatmapInstance.setOption(option, true);
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
