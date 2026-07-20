"use strict";

const $ = (sel) => document.querySelector(sel);

let me = null;
let tenants = [];
let sites = [];
let tests = [];
let agents = [];
let alertRules = [];
let editingTest = null;
let pendingTestForRule = null; // set when jumping to alertcfg with a test preselected

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
  $("#server-version").textContent = me.serverVersion ? "net-lama " + me.serverVersion : "";
  $("#nav-admin-btn").classList.toggle("hidden", !me.isAdmin);
  if (me.isAdmin) {
    tenants = await api("GET", "/api/v1/tenants");
    const sel = $("#tenant-context");
    sel.classList.remove("hidden");
    const prev = sel.value;
    sel.innerHTML = tenants.map((t) => `<option value="${t.id}">${esc(t.name)}</option>`).join("");
    if (prev && tenants.some((t) => t.id === prev)) sel.value = prev;
  }
  // Honor a section deep-link in the URL hash; default to the dashboard.
  const initial = location.hash.slice(1);
  showSection(sections.includes(initial) ? initial : "dashboard");
}

const sections = ["dashboard", "agents", "tests", "sites", "results", "wireless", "path", "alerts", "alertcfg", "logs", "apikeys", "admin"];

function showSection(name, fromHistory = false) {
  for (const sec of sections) $("#section-" + sec).classList.add("hidden");
  $("#section-" + name).classList.remove("hidden");
  document.querySelectorAll(".nav-item").forEach((b) => {
    b.classList.toggle("active", b.dataset.nav === name);
  });
  // Record navigation in browser history so the back/forward (and mouse
  // back) buttons move between sections instead of leaving the app. The
  // first section replaces the entry so "back" from it exits cleanly.
  if (!fromHistory && location.hash !== "#" + name) {
    if (location.hash === "") history.replaceState(null, "", "#" + name);
    else history.pushState(null, "", "#" + name);
  }
  reloadSection(name);
}

window.addEventListener("popstate", () => {
  const name = location.hash.slice(1);
  if (!sections.includes(name)) return;
  if ($("#app-view").classList.contains("hidden")) return; // not logged in
  if (name !== currentSection()) showSection(name, true);
});

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
  if (name === "alertcfg") loadAlertCfg();
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
  renderDashboardSites(sitesData, ov.siteHealth || []);

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

function renderDashboardSites(sitesData, siteHealth) {
  const tbody = $("#db-sites-table tbody");
  tbody.innerHTML = "";
  $("#db-sites-empty").classList.toggle("hidden", sitesData.length > 0);

  for (const site of sitesData) {
    // Health rollup comes from the server: the site's assigned tests judged
    // only against results from this site's own agents.
    const counts = (siteHealth || []).find((sh) => sh.siteId === site.id) || {};
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
    const names = h.agentNames || [];
    const agentCell = names.length === 0
      ? '<span class="muted">—</span>'
      : names.length <= 2
        ? esc(names.join(", "))
        : `<span title="${esc(names.join(", "))}">${esc(names[0])} +${names.length - 1} more</span>`;
    tr.innerHTML = `
      <td><span class="health ${h.status}">${HEALTH_LABEL[h.status]}</span></td>
      <td><strong>${esc(h.name)}</strong></td>
      <td class="muted">${agentCell}</td>
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

  // The endpoint dot is a CSS circle positioned by percentage, not SVG
  // geometry — preserveAspectRatio="none" (needed so the line fills the
  // cell) scales x/y independently, which would render an SVG <circle> as
  // an ellipse. Percentages stay correct under non-uniform scaling.
  const dotRightPct = (padding / width) * 100;
  const dotTopPct = ((padding + innerHeight - ((current - min) / range) * innerHeight) / height) * 100;

  return `<div class="sparkline-wrap">
    <div class="sparkline-chart">
      <svg viewBox="0 0 ${width} ${height}" preserveAspectRatio="none">
        <polyline points="${points}" fill="none" stroke="var(--cat-1)" stroke-width="2" vector-effect="non-scaling-stroke"/>
      </svg>
      <span class="sparkline-dot" style="right: calc(${dotRightPct.toFixed(2)}% - 3px); top: calc(${dotTopPct.toFixed(2)}% - 3px);"></span>
    </div>
    <span class="sparkline-value">${esc(currentText)}</span>
  </div>`;
}

async function renderDashboardWireless(siteId) {
  const tbody = $("#db-wireless-table tbody");
  tbody.innerHTML = "";

  try {
    // Fetch latest wlan_passive results
    const params = new URLSearchParams({ type: "wlan_passive", limit: "100" });
    if (siteId) params.set("siteId", siteId);
    const tid = tenantParam("");
    if (tid) params.set("tenantId", tid.split("=")[1]);
    const results = await api("GET", "/api/v1/results?" + params.toString());

    // Group by agent, keep latest per agent
    const latestByAgent = {};
    for (const r of results) {
      if (!latestByAgent[r.agentId]) {
        latestByAgent[r.agentId] = r;
      }
    }

    const rslt = Object.values(latestByAgent);
    $("#db-wireless-empty").classList.toggle("hidden", rslt.length > 0);

    // Parse payloads and render networks
    for (const result of rslt.slice(0, 20)) { // Show latest 20 agents
      try {
        const payload = typeof result.payload === "string" ? JSON.parse(result.payload) : result.payload;
        const wp = payload.wlanPassive || {};
        const allNetworks = (wp.networks || []).slice().sort((a, b) => (b.rssiDbm || -999) - (a.rssiDbm || -999));

        // Group by SSID (non-empty only)
        const groupedBySSID = new Map();
        const hiddenNetworks = [];
        for (const n of allNetworks) {
          if (!n.ssid) {
            hiddenNetworks.push(n);
          } else {
            if (!groupedBySSID.has(n.ssid)) {
              groupedBySSID.set(n.ssid, []);
            }
            groupedBySSID.get(n.ssid).push(n);
          }
        }

        // Render grouped SSIDs (show top 5)
        let count = 0;
        for (const [ssid, networks] of groupedBySSID.entries()) {
          if (count >= 5) break;
          const strongest = networks[0]; // Already sorted by RSSI
          const rssi = strongest.rssiDbm || 0;
          const sigClass = signalClass(rssi);
          const apCountText = networks.length > 1 ? ` <span class="ap-count">${networks.length} APs</span>` : "";

          const tr = document.createElement("tr");
          tr.classList.add("clickable-row");
          tr.tabIndex = 0;
          tr.innerHTML = `
            <td>${esc(result.agentName)}</td>
            <td>${esc(ssid)}${apCountText}</td>
            <td class="mono">${esc(strongest.bssid)}</td>
            <td class="num"><span class="health ${sigClass}">${rssi}</span> ${renderSignalBar(rssi)}</td>
            <td class="num">${strongest.channel || "—"}</td>
            <td>${strongest.security || "Open"}</td>`;
          tr.addEventListener("click", () => navTo("wireless"));
          tr.addEventListener("keydown", (e) => {
            if (e.key === "Enter") navTo("wireless");
          });
          tbody.appendChild(tr);
          count++;
        }

        // Show hidden networks (not grouped)
        for (const n of hiddenNetworks) {
          if (count >= 5) break;
          const rssi = n.rssiDbm || 0;
          const sigClass = signalClass(rssi);
          const tr = document.createElement("tr");
          tr.classList.add("clickable-row");
          tr.tabIndex = 0;
          tr.innerHTML = `
            <td>${esc(result.agentName)}</td>
            <td><span class="muted">(hidden)</span></td>
            <td class="mono">${esc(n.bssid)}</td>
            <td class="num"><span class="health ${sigClass}">${rssi}</span> ${renderSignalBar(rssi)}</td>
            <td class="num">${n.channel || "—"}</td>
            <td>${n.security || "Open"}</td>`;
          tr.addEventListener("click", () => navTo("wireless"));
          tr.addEventListener("keydown", (e) => {
            if (e.key === "Enter") navTo("wireless");
          });
          tbody.appendChild(tr);
          count++;
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
async function fetchAlertRules() {
  alertRules = await api("GET", "/api/v1/alert-rules" + tenantParam());
  return alertRules;
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
    const capabilityLabel = (c) => c === "wlan" ? "WLAN" : c;
    const caps = (a.capabilities || [])
      .map((c) => `<span class="chip type-${esc(c)}">${capabilityLabel(c)}</span>`).join(" ");
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><span class="badge ${a.connected ? "on" : "off"}">${a.connected ? "connected" : "offline"}</span></td>
      <td><strong>${esc(a.name)}</strong></td>
      <td>${esc(a.siteName)}</td>
      <td class="mono muted">${esc(a.version || "—")}</td>
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
  if (t.type === "wlan_passive") return "passive monitor-mode sweep";
  if (t.type === "wlan_active") {
    const sec = p.security === "eap-peap" ? "802.1X" : p.security === "open" ? "open" : "PSK";
    return `connect to ${p.ssid || "?"} · ${sec}${p.throughputUrl ? " · throughput" : ""}`;
  }
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
  await Promise.all([fetchTests(), fetchAlertRules()]);
  const tbody = $("#tests-table tbody");
  tbody.innerHTML = "";
  $("#tests-empty").classList.toggle("hidden", tests.length > 0);
  for (const t of tests) {
    const rulesForTest = alertRules.filter((r) => r.testId === t.id);
    const ruleNames = rulesForTest.length > 0
      ? esc(rulesForTest.map((r) => r.name).join(", "))
      : '<span class="muted">—</span>';
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><strong>${esc(t.name)}</strong></td>
      <td><span class="chip type-${t.type}">${t.type}</span></td>
      <td class="muted">${t.intervalSeconds}s</td>
      <td class="muted">${esc(paramsSummary(t))}</td>
      <td class="muted">${ruleNames}</td>
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
  $("#t-params-wlan_passive").classList.toggle("hidden", type !== "wlan_passive");
  $("#t-params-wlan_active").classList.toggle("hidden", type !== "wlan_active");
  updateWlanActiveSecurityFields();
  $("#t-params-traceroute").classList.toggle("hidden", type !== "traceroute");
  $("#t-params-speedtest").classList.toggle("hidden", type !== "speedtest");
  // Re-filter alert rules when test type changes
  populateAlertRuleSelect(type);
}
// Show identity + certificate fields only for EAP, password only when needed
function updateWlanActiveSecurityFields() {
  const sec = $("#t-wa-security").value;
  $("#t-wa-identity-label").classList.toggle("hidden", sec !== "eap-peap");
  $("#t-wa-password-label").classList.toggle("hidden", sec === "open");
  $("#t-wa-eap-extra").classList.toggle("hidden", sec !== "eap-peap");
}
$("#t-wa-security").addEventListener("change", updateWlanActiveSecurityFields);
$("#t-wa-macmode").addEventListener("change", () => {
  $("#t-wa-mac-warn").classList.toggle("hidden", $("#t-wa-macmode").value !== "random");
});

$("#t-type").addEventListener("change", () => {
  updateTestParamFields();
  // Direction and unit change with the type — reset the bands for the new type
  initThresholdBands($("#t-type").value, {});
});

function populateAlertRuleSelect(testType) {
  const applicableRules = getApplicableRules(testType);
  const select = $("#t-alert-rule");
  const label = $("#t-alert-rule-label");
  const createBtn = $("#t-create-alert-rule");
  const section = $("#t-alert-rule-section");

  if (alertRules.length === 0) {
    // No rules exist: show create button
    select.parentElement.style.display = "none";
    createBtn.style.display = "inline-block";
  } else {
    // Rules exist: populate select with applicable rules
    select.parentElement.style.display = "block";
    createBtn.style.display = "none";
    select.innerHTML = '<option value="">— none —</option>' +
      applicableRules.map((r) => `<option value="${r.id}">${esc(r.name)}</option>`).join("");
  }
}

// --- State-threshold band editor (Grafana-style colored bands) ---
// Backed by the same {warn, crit} model: warn is the green|orange boundary,
// crit the orange|red one. For speedtest lower is worse, so the band order
// flips (green on top) and warn > crit.
const BAND_UNIT = { ping: "ms", dns: "ms", http: "ms", tcp: "ms", speedtest: "Mbps", traceroute: "hops", wlan_passive: "%", wlan_active: "ms" };
// null = band absent, "" = band added but value not typed yet, "40" = value
let bandTh = { warn: null, crit: null };
let bandType = "ping";

function initThresholdBands(type, th) {
  bandType = type;
  bandTh.warn = th.warn != null ? String(th.warn) : null;
  bandTh.crit = th.crit != null ? String(th.crit) : null;
  renderBands();
}

// bandRows returns the rows to draw, highest boundary first. Each row:
// {color, field (editable boundary; null for the catch-all band), text}.
function bandRows() {
  const lw = bandType === "speedtest"; // lower is worse
  const hasW = bandTh.warn !== null, hasC = bandTh.crit !== null;
  const w = bandTh.warn || "…", c = bandTh.crit || "…";
  if (hasW && hasC) {
    return lw
      ? [{ color: "green", field: "warn", text: "and greater" },
         { color: "orange", field: "crit", text: `to ${w}` },
         { color: "red", field: null, text: `less than ${c}` }]
      : [{ color: "red", field: "crit", text: "and greater" },
         { color: "orange", field: "warn", text: `to ${c}` },
         { color: "green", field: null, text: `less than ${w}` }];
  }
  if (hasW) {
    return lw
      ? [{ color: "green", field: "warn", text: "and greater" },
         { color: "orange", field: null, text: `less than ${w}` }]
      : [{ color: "orange", field: "warn", text: "and greater" },
         { color: "green", field: null, text: `less than ${w}` }];
  }
  if (hasC) {
    return lw
      ? [{ color: "green", field: "crit", text: "and greater" },
         { color: "red", field: null, text: `less than ${c}` }]
      : [{ color: "red", field: "crit", text: "and greater" },
         { color: "green", field: null, text: `less than ${c}` }];
  }
  return [];
}

function renderBands() {
  const unit = BAND_UNIT[bandType] || "";
  $("#t-band-unit").textContent = unit ? `(${unit}, optional)` : "(optional)";
  const box = $("#t-bands");
  box.innerHTML = "";

  for (const row of bandRows()) {
    const div = document.createElement("div");
    div.className = "band-row";
    div.innerHTML = `<span class="band-swatch ${row.color}"></span>` +
      (row.field ? `<input type="number" step="any" data-band="${row.field}" value="${bandTh[row.field]}">` : "") +
      `<span class="band-text" data-band-text>${row.text}</span>` +
      `<button type="button" class="ghost band-x" data-band-x="${row.field || "all"}" title="Remove">✕</button>`;
    box.appendChild(div);
  }

  // Add buttons for missing bands (warn ↔ orange, crit ↔ red)
  const add = document.createElement("div");
  add.className = "band-add";
  if (bandTh.warn === null && bandTh.crit === null) {
    add.innerHTML = `<button type="button" class="ghost" data-band-add="both">+ Add state thresholds</button>`;
  } else {
    if (bandTh.warn === null) add.innerHTML += `<button type="button" class="ghost" data-band-add="warn">+ Degraded (orange) state</button>`;
    if (bandTh.crit === null) add.innerHTML += `<button type="button" class="ghost" data-band-add="crit">+ Failing (red) state</button>`;
  }
  if (add.innerHTML) box.appendChild(add);
}

$("#t-bands").addEventListener("input", (e) => {
  const f = e.target.dataset.band;
  if (!f) return;
  bandTh[f] = e.target.value;
  // Update dependent range labels without re-rendering (keeps focus)
  const texts = $("#t-bands").querySelectorAll("[data-band-text]");
  bandRows().forEach((row, i) => { if (texts[i]) texts[i].textContent = row.text; });
});

$("#t-bands").addEventListener("click", (e) => {
  const x = e.target.dataset.bandX, a = e.target.dataset.bandAdd;
  if (!x && !a) return;
  if (x === "all") { bandTh.warn = null; bandTh.crit = null; }
  else if (x) bandTh[x] = null;
  if (a === "both") { bandTh.warn = ""; bandTh.crit = ""; }
  else if (a) bandTh[a] = "";
  renderBands();
  // Focus the first empty input of the freshly added band
  const inp = $("#t-bands").querySelector('input[data-band][value=""]');
  if (inp) inp.focus();
});

// readThresholdBands returns {warn?, crit?}, null for none, or undefined
// after showing a validation error.
function readThresholdBands() {
  const w = bandTh.warn, c = bandTh.crit;
  if (w === null && c === null) return null;
  if (w === "" || c === "" || (w !== null && isNaN(+w)) || (c !== null && isNaN(+c))) {
    dialogError("#t-error", "Fill in every state boundary or remove the band");
    return undefined;
  }
  const th = {};
  if (w !== null) th.warn = +w;
  if (c !== null) th.crit = +c;
  if (th.warn != null && th.crit != null) {
    const lw = bandType === "speedtest";
    if (lw ? th.warn <= th.crit : th.warn >= th.crit) {
      dialogError("#t-error", "State boundaries overlap — adjust the band values");
      return undefined;
    }
  }
  return th;
}

function openTestDialog(test) {
  editingTest = test || null;
  $("#test-dlg-title").textContent = test ? `Edit "${test.name}"` : "New test";
  $("#t-submit").textContent = test ? "Save" : "Create";
  $("#t-type").disabled = !!test;
  dialogError("#t-error", "");

  $("#t-name").value = test ? test.name : "";
  const testType = test ? test.type : "ping";
  $("#t-type").value = testType;
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
  const wa = (test && test.type === "wlan_active") ? p : {};
  $("#t-wa-ssid").value = wa.ssid || "";
  $("#t-wa-security").value = wa.security || "psk";
  $("#t-wa-password").value = wa.password || "";
  $("#t-wa-identity").value = wa.identity || "";
  $("#t-wa-cacert").value = wa.caCertPem || "";
  $("#t-wa-insecure").checked = !!wa.insecureSkipVerify;
  $("#t-wa-tpurl").value = wa.throughputUrl || "";
  $("#t-wa-macmode").value = wa.macMode || "permanent";
  $("#t-wa-mac-warn").classList.toggle("hidden", ($("#t-wa-macmode").value) !== "random");
  updateTestParamFields();

  // State thresholds band editor
  initThresholdBands(testType, (test && test.thresholds) || {});

  // Populate alert rule selection
  populateAlertRuleSelect(testType);
  if (test) {
    const rulesForTest = alertRules.filter((r) => r.testId === test.id);
    if (rulesForTest.length > 0) {
      $("#t-alert-rule").value = rulesForTest[0].id;
    }
  }

  $("#dlg-test").showModal();
}

$("#btn-new-test").addEventListener("click", () => openTestDialog(null));

$("#t-create-alert-rule").addEventListener("click", async () => {
  // Close the test dialog
  $("#dlg-test").close();
  // If editing an existing test, preselect it in the rule dialog
  if (editingTest) {
    pendingTestForRule = editingTest.id;
  }
  // Navigate to alertcfg and open the new rule dialog
  navTo("alertcfg");
});

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
  } else if (type === "wlan_passive") {
    params = {};
  } else if (type === "wlan_active") {
    params = {
      ssid: $("#t-wa-ssid").value.trim(),
      security: $("#t-wa-security").value,
      password: $("#t-wa-password").value,
      identity: $("#t-wa-identity").value.trim(),
      caCertPem: $("#t-wa-cacert").value.trim(),
      insecureSkipVerify: $("#t-wa-insecure").checked,
      throughputUrl: $("#t-wa-tpurl").value.trim(),
      macMode: $("#t-wa-macmode").value,
    };
  }
  // Build thresholds from the band editor (undefined => validation error)
  const thresholds = readThresholdBands();
  if (thresholds === undefined) return;

  const body = {
    name: $("#t-name").value.trim(),
    type,
    intervalSeconds: +$("#t-interval").value,
    params,
  };
  if (thresholds) body.thresholds = thresholds;
  try {
    let testId;
    if (editingTest) {
      await api("PUT", `/api/v1/tests/${editingTest.id}`, body);
      testId = editingTest.id;
    } else {
      const tid = tenantParam("");
      if (tid) body.tenantId = tid.split("=")[1];
      const res = await api("POST", "/api/v1/tests", body);
      testId = res.id;
    }

    // Handle alert rule reassignment
    const selectedRuleId = $("#t-alert-rule").value;
    if (selectedRuleId) {
      const rule = alertRules.find((r) => r.id === selectedRuleId);
      if (rule && rule.testId !== testId) {
        // Re-point the rule to this test
        const ruleUpdate = {
          name: rule.name,
          testId,
          metric: rule.metric,
          operator: rule.operator,
          threshold: rule.threshold,
          forCount: rule.forCount,
          clearThreshold: rule.clearThreshold,
          clearCount: rule.clearCount,
          targetIds: rule.targetIds,
        };
        await api("PUT", `/api/v1/alert-rules/${selectedRuleId}`, ruleUpdate);
      }
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
// Maps test type to required capability (e.g., wlan_passive → wlan).
function requiredCapability(testType) {
  if (testType === "wlan_passive") return "wlan";
  if (testType === "wlan_active") return "wlan_active";
  return testType;
}

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
      const reqCap = requiredCapability(t.type);
      if (caps.length && !caps.includes(reqCap)) {
        warnings.push(`${t.name} won't run on ${a.name} (no ${reqCap} capability)`);
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
  if (p.wlanPassive) {
    const n = (p.wlanPassive.networks || []).length;
    const m = (p.wlanPassive.stations || []).length;
    return `${n} network${n === 1 ? "" : "s"}, ${m} client${m === 1 ? "" : "s"}`;
  }
  if (p.wlanActive) {
    const w = p.wlanActive;
    if (!w.success) {
      return `${esc(w.ssid)} · <span class="error">${esc(w.failedStep || "failed")}</span>` +
        (w.associateMs ? ` · assoc ${fmt(w.associateMs)} ms` : "");
    }
    const ip = w.ip ? esc(w.ip) + (w.netmask ? "/" + netmaskToPrefix(w.netmask) : "") : "?";
    let out = `${esc(w.ssid)} · assoc ${fmt(w.associateMs)} ms · auth ${fmt(w.authenticateMs)} ms · dhcp ${fmt(w.dhcpMs)} ms · ${ip}`;
    if (w.gateway) out += ` gw ${esc(w.gateway)}`;
    if (w.gatewayPingRttMs) out += ` · ping ${fmt(w.gatewayPingLossPct)}% loss/${fmt(w.gatewayPingRttMs)} ms`;
    if (w.throughputMbps) out += ` · ${fmt(w.throughputMbps)} Mbps`;
    return out;
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
  return style.getPropertyValue(`--cat-${(i % 8) + 1}`).trim();
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
  // Check if any wlan_passive tests exist
  const tests = await api("GET", "/api/v1/tests" + tenantParam());
  const hasWlanPassive = tests.some((t) => t.type === "wlan_passive");

  if (!hasWlanPassive) {
    $("#wl-notest").classList.remove("hidden");
    $("#wl-body").classList.add("hidden");
    return;
  }

  $("#wl-notest").classList.add("hidden");

  // Get WLAN-capable agents
  wlAgents = await api("GET", "/api/v1/agents" + tenantParam());
  wlAgents = wlAgents.filter((a) => (a.capabilities || []).includes("wlan"));

  const sel = $("#wl-agent");
  const prev = sel.value;
  sel.innerHTML = wlAgents.map((a) => `<option value="${a.id}">${esc(a.name)} (${esc(a.siteName)})</option>`).join("");
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

  // Latest wlan_passive result for this agent
  const params = new URLSearchParams({ agentId: agent.id, type: "wlan_passive", limit: "1" });
  const tid = tenantParam("");
  if (tid) params.set("tenantId", tid.split("=")[1]);
  const results = await api("GET", "/api/v1/results?" + params.toString());
  renderWirelessNetworks(results[0]);
  renderWirelessActive(agent);
  renderWirelessRoaming(agent);
}

// netmaskToPrefix converts a dotted netmask ("255.255.255.0") to a prefix
// length (24). Falls back to the raw string if it doesn't parse.
function netmaskToPrefix(mask) {
  const octets = mask.split(".").map(Number);
  if (octets.length !== 4 || octets.some((o) => isNaN(o))) return mask;
  return octets.reduce((bits, o) => bits + ((o >>> 0).toString(2).match(/1/g) || []).length, 0);
}

// --- Active connection test (wlan_active) on the Wireless page ---
const gatewayPingCount = 20; // must match gatewayPingCount in internal/probe/wlanactive_linux.go
let wlActiveChart = null;
let wlActiveLast = null; // latest wlan_active payload, for theme re-render

// renderWirelessActive shows the active-test card with a step waterfall when
// the selected agent has wlan_active results; hidden otherwise.
async function renderWirelessActive(agent) {
  const card = $("#wl-active");
  let r = null;
  try {
    const params = new URLSearchParams({ agentId: agent.id, type: "wlan_active", limit: "1" });
    const tid = tenantParam("");
    if (tid) params.set("tenantId", tid.split("=")[1]);
    r = (await api("GET", "/api/v1/results?" + params.toString()))[0];
  } catch (e) { /* treat as no data */ }
  const wa = r && r.payload && r.payload.wlanActive;
  if (!wa) {
    card.classList.add("hidden");
    wlActiveLast = null;
    return;
  }
  wlActiveLast = wa;
  card.classList.remove("hidden");

  const meta = $("#wl-active-meta");
  meta.textContent = "";
  if (wa.demo) {
    const badge = document.createElement("span");
    badge.className = "demo-badge";
    badge.textContent = "DEMO DATA";
    meta.appendChild(badge);
  }
  meta.appendChild(document.createTextNode(
    `${r.testName} · ${new Date(r.time).toLocaleString()}`));

  const status = wa.success
    ? '<span class="health healthy">connected</span>'
    : `<span class="health failing">failed: ${esc(wa.failedStep || r.error || "unknown")}</span>`;
  const cidr = wa.ip ? esc(wa.ip) + (wa.netmask ? "/" + netmaskToPrefix(wa.netmask) : "") : "";
  const dns = (wa.dnsServers && wa.dnsServers.length)
    ? wa.dnsServers.map((d) => `<span class="mono">${esc(d)}</span>`).join(", ")
    : "—";
  const signal = wa.rssiDbm
    ? `<span class="health ${signalClass(wa.rssiDbm)}">${wa.rssiDbm} dBm</span>` +
      (wa.snrDb ? ` <span class="muted">SNR ${fmt(wa.snrDb, 0)} dB</span>` : "")
    : "—";
  // txRetryPct = retries / (packets + retries); with no throughputUrl the
  // only traffic is the DHCP handshake (~10-15 frames), where a single
  // retry swings the result by several points — flag that low a sample.
  // A gateway ping (20 echoes) runs automatically after DHCP to guarantee
  // real traffic, so most runs clear this; a small sample here usually means
  // the ping didn't run (e.g. no router option in the lease).
  const txAttempts = (wa.txPackets || 0) + (wa.txRetries || 0);
  const retrans = wa.txPackets
    ? `${fmt(wa.txRetryPct)}% <span class="muted">(${wa.txRetries}/${txAttempts} attempts)</span>` +
      (txAttempts < 25 ? ' <span class="muted">— small sample</span>' : "")
    : "—";
  const gwPing = wa.gateway && wa.gatewayPingRttMs
    ? `${fmt(wa.gatewayPingLossPct)}% loss, ${fmt(wa.gatewayPingRttMs)} ms avg <span class="muted">(${gatewayPingCount} pings)</span>`
    : "—";
  const rows = [
    ["SSID", esc(wa.ssid || "—") + (wa.bssid ? ` <span class="muted mono">${esc(wa.bssid)}</span>` : "")],
    ["Status", status],
    ["IP address", cidr ? `<span class="mono">${cidr}</span>` : "—"],
    ["Client MAC", wa.mac ? `<span class="mono">${esc(wa.mac)}</span>` : "—"],
    ["Gateway", wa.gateway ? `<span class="mono">${esc(wa.gateway)}</span>` : "—"],
    ["Gateway ping", gwPing],
    ["DNS servers", dns],
    ["Signal", signal],
    ["TX retransmits", retrans],
    ["Throughput", wa.throughputMbps ? `${fmt(wa.throughputMbps)} Mbps` : "—"],
    ["Connect time", `${fmt((wa.associateMs || 0) + (wa.authenticateMs || 0) + (wa.dhcpMs || 0))} ms`],
  ];
  $("#wl-active-summary").innerHTML = rows
    .map(([k, v]) => `<div class="wl-detail-item"><span class="wl-detail-label">${k}</span><span class="wl-detail-value">${v}</span></div>`)
    .join("");

  renderWlActiveWaterfall(wa);
}

// renderWlActiveWaterfall draws the per-step connection waterfall:
// association → authentication → DHCP (+ throughput when measured), each
// bar offset by the cumulative time of the previous steps.
function renderWlActiveWaterfall(wa) {
  const container = $("#wl-active-waterfall");
  // scanMs stays payload-only: SSID discovery is harness-internal, not a
  // connection-quality signal
  const steps = [
    { name: "Association", ms: wa.associateMs || 0, key: "associate" },
    { name: "Authentication", ms: wa.authenticateMs || 0, key: "authenticate" },
    { name: "DHCP", ms: wa.dhcpMs || 0, key: "dhcp" },
  ];
  if (wa.throughputMs) {
    steps.push({ name: "Throughput", ms: wa.throughputMs, key: "throughput" });
  }

  const style = getComputedStyle(document.documentElement);
  const accentColor = style.getPropertyValue("--accent").trim();
  const badColor = style.getPropertyValue("--bad").trim();
  const mutedColor = style.getPropertyValue("--muted-solid").trim();
  const textColor = style.getPropertyValue("--fg").trim();

  const baseData = [];
  const barData = [];
  let cumulative = 0;
  for (const s of steps) {
    baseData.push(cumulative);
    barData.push({
      value: s.ms,
      itemStyle: { color: wa.failedStep === s.key ? badColor : accentColor },
    });
    cumulative += s.ms;
  }
  const maxNice = niceMax(Math.max(cumulative, 1) * 1.1);

  const option = {
    tooltip: {
      trigger: "axis",
      axisPointer: { type: "shadow" },
      formatter: function (params) {
        if (!params || !params.length) return "";
        const i = params[0].dataIndex;
        const s = steps[i];
        if (wa.failedStep === s.key) {
          return `<strong>${s.name}</strong><br/>failed after ${fmt(s.ms)} ms`;
        }
        return `<strong>${s.name}</strong>: ${fmt(s.ms)} ms<br/>` +
          `completed at ${fmt(baseData[i] + s.ms)} ms`;
      },
    },
    grid: { top: 44, bottom: 24, left: 110, right: 30 },
    xAxis: {
      type: "value",
      position: "top",
      min: 0,
      max: maxNice,
      name: "ms",
      nameGap: 6,
      nameTextStyle: { color: mutedColor, fontSize: 11 },
      axisLabel: { color: mutedColor, fontSize: 11 },
      axisLine: { lineStyle: { color: mutedColor } },
      splitLine: { lineStyle: { color: mutedColor, opacity: 0.2 } },
    },
    yAxis: {
      type: "category",
      data: steps.map((s) => s.name),
      inverse: true,
      axisLabel: { color: mutedColor, fontSize: 11 },
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
        name: "Duration",
        type: "bar",
        stack: "waterfall",
        barWidth: 16,
        data: barData,
        itemStyle: { borderWidth: 0 },
      },
    ],
    textStyle: { color: textColor },
  };

  const chartHeight = Math.max(160, steps.length * 30 + 90);
  if (!wlActiveChart) {
    container.style.height = chartHeight + "px";
    wlActiveChart = window.echarts.init(container);
  } else {
    container.style.height = chartHeight + "px";
    wlActiveChart.resize();
  }
  wlActiveChart.setOption(option, true);
}

window.addEventListener("resize", () => {
  if (wlActiveChart && currentSection() === "wireless") wlActiveChart.resize();
});

// --- Roaming (Meraki-style analytics) on the Wireless page ---
let wlRoamSummary = null; // last fetched summary, for the client dropdown + timeline
let wlRoamSelectedClient = null;

// wlRoamBand approximates band from a channel number — roam events carry
// channel, not frequency, so 6 GHz (which shares numbering with 5 GHz in
// some regional plans) reads as "5 GHz" here. Good enough for the roaming
// view; the Nearby networks table has the precise per-network band.
function wlRoamBand(channel) {
  if (!channel) return "—";
  return channel <= 14 ? "2.4 GHz" : "5 GHz";
}

function wlRoamDuration(ms) {
  if (!ms || ms <= 0) return "—";
  const s = Math.round(ms / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${s % 60}s`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m`;
}

$("#wl-roam-range").addEventListener("change", () => renderWirelessRoaming(currentWlAgent()));
$("#wl-roam-client").addEventListener("change", () => {
  wlRoamSelectedClient = $("#wl-roam-client").value;
  renderWlRoamTimeline();
});

async function renderWirelessRoaming(agent) {
  if (!agent) return;
  const days = +$("#wl-roam-range").value || 7;
  const since = new Date(Date.now() - days * 24 * 60 * 60 * 1000).toISOString();
  const params = new URLSearchParams({ agentId: agent.id, since });
  const tid = tenantParam("");
  if (tid) params.set("tenantId", tid.split("=")[1]);

  let summary;
  try {
    summary = await api("GET", "/api/v1/wlan-roaming?" + params.toString());
  } catch (e) {
    return;
  }
  wlRoamSummary = summary;

  $("#wl-roam-bad").textContent = summary.badRoams;
  $("#wl-roam-pingpong").textContent = summary.pingPongClients;
  $("#wl-roam-sticky").textContent = summary.stickyClients;
  $("#wl-roam-suboptimal").textContent = summary.suboptimalRoams;
  $("#wl-roam-good").textContent = summary.goodRoams;
  $("#wl-roam-disconnects").textContent = summary.disconnects;

  const events = summary.events || [];
  $("#wl-roam-empty").classList.toggle("hidden", events.length > 0);
  $("#wl-roam-timeline-wrap").classList.toggle("hidden", events.length === 0);

  // Client dropdown: top clients by event count, most active first
  const countByClient = new Map();
  for (const e of events) countByClient.set(e.clientMac, (countByClient.get(e.clientMac) || 0) + 1);
  const clients = [...countByClient.entries()].sort((a, b) => b[1] - a[1]).map(([mac]) => mac);
  if (!wlRoamSelectedClient || !clients.includes(wlRoamSelectedClient)) {
    wlRoamSelectedClient = clients[0] || null;
  }
  $("#wl-roam-client").innerHTML = clients
    .map((mac) => `<option value="${esc(mac)}"${mac === wlRoamSelectedClient ? " selected" : ""}>${esc(mac)} (${countByClient.get(mac)} events)</option>`)
    .join("");
  renderWlRoamTimeline();

  // Event log table
  const tbody = $("#wl-roam-table tbody");
  tbody.innerHTML = events
    .map((e) => {
      const cls = e.classification || "";
      const badge = cls ? `<span class="wl-roam-badge ${cls}" title="${cls}"></span>` : "";
      const route = e.toBssid
        ? `<span class="mono">${esc(e.fromBssid || "—")}</span> → <span class="mono">${esc(e.toBssid)}</span>`
        : `<span class="mono">${esc(e.fromBssid)}</span> → <span class="error">disconnected</span>`;
      const roamTime = e.toBssid ? fmt(e.roamTimeMs, 0) : "—";
      const rssi = e.fromRssiDbm && e.toRssiDbm ? `${e.fromRssiDbm} → ${e.toRssiDbm}` : (e.toRssiDbm || e.fromRssiDbm || "—");
      const band = wlRoamBand(e.toChannel || e.fromChannel);
      return `<tr>
        <td>${badge}</td>
        <td>${esc(e.clientMac)}${e.ssid ? ` <span class="muted">(${esc(e.ssid)})</span>` : ""}</td>
        <td>${route}</td>
        <td class="num">${roamTime}</td>
        <td class="num">${rssi}</td>
        <td>${band}</td>
        <td class="muted nowrap">${new Date(e.detectedAtMs).toLocaleString()}</td>
        <td class="muted nowrap">${wlRoamDuration(e.durationMs)}</td>
      </tr>`;
    })
    .join("");
}

// renderWlRoamTimeline draws a per-AP swimlane for the selected client:
// one row per BSSID it visited, a horizontal segment for each span of time
// connected to that AP, and a colored dot at each roam-in transition.
function renderWlRoamTimeline() {
  const container = $("#wl-roam-timeline");
  container.innerHTML = "";
  const events = (wlRoamSummary && wlRoamSummary.events) || [];
  const clientEvents = events.filter((e) => e.clientMac === wlRoamSelectedClient).slice().reverse(); // oldest first
  if (!clientEvents.length) return;

  const rangeStart = clientEvents[0].detectedAtMs;
  const rangeEnd = Date.now();
  const span = Math.max(1, rangeEnd - rangeStart);
  const pct = (ms) => Math.min(100, Math.max(0, ((ms - rangeStart) / span) * 100));

  // One lane per distinct BSSID this client visited
  const bssids = [...new Set(clientEvents.filter((e) => e.toBssid).map((e) => e.toBssid))];
  for (const bssid of bssids) {
    const lane = document.createElement("div");
    lane.className = "wl-roam-lane";
    const label = document.createElement("div");
    label.className = "wl-roam-lane-label";
    label.textContent = bssid;
    label.title = bssid;
    const track = document.createElement("div");
    track.className = "wl-roam-lane-track";

    for (let i = 0; i < clientEvents.length; i++) {
      const e = clientEvents[i];
      if (e.toBssid !== bssid) continue;
      const segEnd = e.durationMs ? e.detectedAtMs + e.durationMs : rangeEnd;
      const seg = document.createElement("div");
      seg.className = "wl-roam-segment";
      seg.style.left = pct(e.detectedAtMs) + "%";
      seg.style.width = Math.max(0.5, pct(segEnd) - pct(e.detectedAtMs)) + "%";
      seg.title = `${new Date(e.detectedAtMs).toLocaleString()} – ${wlRoamDuration(e.durationMs) || "now"}`;
      track.appendChild(seg);

      if (e.classification) {
        const dot = document.createElement("div");
        dot.className = "wl-roam-dot " + e.classification;
        dot.style.left = pct(e.detectedAtMs) + "%";
        dot.title = `${e.fromBssid} → ${e.toBssid} (${e.classification}, ${fmt(e.roamTimeMs, 0)} ms)`;
        track.appendChild(dot);
      }
    }
    lane.appendChild(label);
    lane.appendChild(track);
    container.appendChild(lane);
  }

  const axis = document.createElement("div");
  axis.className = "wl-roam-axis";
  const axisLabel = document.createElement("div");
  const axisTrack = document.createElement("div");
  axisTrack.className = "wl-roam-axis-track";
  const startTick = document.createElement("span");
  startTick.className = "wl-roam-axis-tick";
  startTick.style.left = "0%";
  startTick.textContent = new Date(rangeStart).toLocaleString();
  const endTick = document.createElement("span");
  endTick.className = "wl-roam-axis-tick";
  endTick.style.left = "100%";
  endTick.textContent = "now";
  axisTrack.appendChild(startTick);
  axisTrack.appendChild(endTick);
  axis.appendChild(axisLabel);
  axis.appendChild(axisTrack);
  container.appendChild(axis);
}

// signalClass buckets an RSSI (dBm) into a health color: >=-60 ok,
// -75..-60 warn, <-75 bad.
function signalClass(rssi) {
  if (rssi >= -60) return "healthy";
  if (rssi >= -75) return "degraded";
  return "failing";
}

// signalBarPercent converts RSSI (dBm) to a 0-100% fill level.
// Scale: -90 dBm → 0%, -30 dBm → 100% (clamped).
function signalBarPercent(rssi) {
  if (!rssi) rssi = -90;
  const percent = ((rssi + 90) / 60) * 100;
  return Math.max(0, Math.min(100, percent));
}

// renderSignalBar creates a signal strength bar HTML element.
function renderSignalBar(rssi) {
  const percent = signalBarPercent(rssi);
  const color = signalClass(rssi);
  return `<div class="signal-bar"><div class="signal-fill ${color}" style="width: ${percent}%"></div></div>`;
}

// --- AP vendor lookup (server resolves BSSID OUIs from the IEEE registry) ---
const ouiCache = new Map();
async function lookupVendors(macs) {
  const missing = [...new Set(macs)].filter((m) => m && !ouiCache.has(m));
  if (missing.length) {
    try {
      const res = await api("GET", "/api/v1/oui?macs=" + encodeURIComponent(missing.join(",")));
      for (const m of missing) ouiCache.set(m, res[m] || "");
    } catch (e) { /* vendors stay unresolved */ }
  }
  return macs.map((m) => ouiCache.get(m) || "");
}

// toMs coerces a protojson int64 (serialized as a string) to a number.
const toMs = (v) => Number(v) || 0;

// fillVendors resolves and fills all .wl-vendor placeholders under rootSel.
async function fillVendors(rootSel) {
  const els = [...document.querySelectorAll(rootSel + " .wl-vendor")];
  await lookupVendors(els.map((el) => el.dataset.mac));
  for (const el of els) {
    const v = ouiCache.get(el.dataset.mac) || "";
    el.textContent = v || "unknown";
    el.classList.toggle("muted", !v);
  }
}

let wlDetailBssid = null; // BSSID of the open detail panel, null = closed
let wlLastResult = null; // latest wlan_passive result, for filter re-renders

$("#wl-detail-close").addEventListener("click", () => {
  wlDetailBssid = null;
  $("#wl-detail").classList.add("hidden");
});

for (const id of ["wl-filter-ssid", "wl-band-24", "wl-band-5", "wl-band-6"]) {
  $("#" + id).addEventListener("input", () => renderWirelessNetworks(wlLastResult));
}

// showApDetail fills the AP detail panel for one network from the latest sweep.
async function showApDetail(n, wp, result) {
  wlDetailBssid = n.bssid;
  $("#wl-detail").classList.remove("hidden");
  $("#wl-detail-title").textContent = (n.ssid || "(hidden)") + " — " + n.bssid;

  const freq = n.freqMhz || 0;
  const band = freq >= 5955 ? "6 GHz" : freq >= 5000 ? "5 GHz" : "2.4 GHz";
  const roamNames = {
    k: "Radio Measurement (802.11k)",
    r: "Fast BSS Transition (802.11r)",
    v: "BSS Transition Mgmt (802.11v)",
  };
  const roaming = n.roaming
    ? n.roaming.split("/").map((x) => roamNames[x] || esc(x)).join("<br>")
    : "—";
  const mfp = n.mfp === "required" ? "Required (802.11w)"
    : n.mfp === "capable" ? "Capable (802.11w)"
    : "Not supported";
  const rows = [
    ["Vendor", '<span class="wl-vendor muted" data-mac="' + esc(n.bssid) + '">resolving…</span>'],
    ["Signal", `<span class="health ${signalClass(n.rssiDbm)}">${n.rssiDbm || 0} dBm</span> ${renderSignalBar(n.rssiDbm)}`],
    ["Channel", `${n.channel || "—"} · ${band}${n.widthMhz ? ` · ${n.widthMhz} MHz` : ""}`],
    ["Frequency", freq ? freq + " MHz" : "—"],
    ["Security", esc(n.security || "Open") + (n.securityDetail ? ` <span class="muted">(${esc(n.securityDetail)})</span>` : "")],
    ["Group cipher", n.groupCipher ? esc(n.groupCipher) : "—"],
    ["Mgmt frame protection", mfp],
    ["WPS", n.wps ? '<span class="health degraded">Enabled</span>' : "—"],
    ["Standards", n.standards ? "802.11 " + esc(n.standards) : "—"],
    ["Spatial streams", n.streams ? `${n.streams}×${n.streams}` : "—"],
    ["Max PHY rate", n.maxRateMbps ? `~${n.maxRateMbps >= 100 ? n.maxRateMbps.toFixed(0) : n.maxRateMbps.toFixed(1)} Mbps` : "—"],
    ["Roaming", roaming],
    ["Beacon interval", n.beaconIntervalTu ? `${n.beaconIntervalTu} TU (${(n.beaconIntervalTu * 1.024).toFixed(0)} ms)` : "—"],
    ["DTIM period", n.dtimPeriod ? String(n.dtimPeriod) : "—"],
    ["Country", n.country ? esc(n.country) : "—"],
    ["AP load", n.loadPresent ? `${n.loadStations || 0} stations · ${(n.loadChannelUtilPct || 0).toFixed(0)}% channel busy` : "—"],
    ["Beacons heard", String(n.beacons || 0)],
    ["Last seen", toMs(n.lastSeenMs) ? new Date(toMs(n.lastSeenMs)).toLocaleString() : result ? new Date(result.time).toLocaleString() : "—"],
  ];
  $("#wl-detail-grid").innerHTML = rows
    .map(([k, v]) => `<div class="wl-detail-item"><span class="wl-detail-label">${k}</span><span class="wl-detail-value">${v}</span></div>`)
    .join("");

  // Clients observed on this BSSID during the sweep
  const clients = ((wp && wp.stations) || []).filter((s) => s.bssid === n.bssid);
  $("#wl-detail-clients-wrap").classList.toggle("hidden", clients.length === 0);
  const ctbody = $("#wl-detail-clients tbody");
  ctbody.innerHTML = clients
    .map((c) => `<tr>
      <td class="mono">${esc(c.mac)}</td>
      <td><span class="wl-vendor muted" data-mac="${esc(c.mac)}"></span></td>
      <td class="num"><span class="health ${signalClass(c.rssiDbm)}">${c.rssiDbm || 0}</span></td>
      <td class="num">${c.rateMbps ? c.rateMbps.toFixed(1) : "—"}</td>
      <td class="num">${c.frames || 0}</td>
      <td class="muted nowrap">${toMs(c.lastSeenMs) ? new Date(toMs(c.lastSeenMs)).toLocaleTimeString() : "—"}</td>
    </tr>`)
    .join("");

  await fillVendors("#wl-detail");
}

function renderWirelessNetworks(result) {
  wlLastResult = result;
  const tbody = $("#wl-networks-table tbody");
  tbody.innerHTML = "";

  const wp = result && result.payload && result.payload.wlanPassive;
  const total = (wp && wp.networks) || [];

  // SSID and band filters
  const q = $("#wl-filter-ssid").value.trim().toLowerCase();
  const bandOK = {
    "2.4": $("#wl-band-24").checked,
    "5": $("#wl-band-5").checked,
    "6": $("#wl-band-6").checked,
  };
  const bandOf = (freq) => (freq >= 5955 ? "6" : freq >= 5000 ? "5" : "2.4");
  const allNetworks = total
    .filter((n) => bandOK[bandOf(n.freqMhz || 0)] && (!q || (n.ssid || "").toLowerCase().includes(q)))
    .sort((a, b) => (b.rssiDbm || -999) - (a.rssiDbm || -999));

  const meta = $("#wl-meta");
  meta.textContent = "";
  if (result) {
    if (wp && wp.demo) {
      const badge = document.createElement("span");
      badge.className = "demo-badge";
      badge.textContent = "DEMO DATA";
      meta.appendChild(badge);
    }
    if (result.error) {
      meta.appendChild(document.createTextNode("Last sweep failed: " + result.error));
    } else if (wp) {
      const count = allNetworks.length === total.length ? `${total.length} networks` : `${allNetworks.length}/${total.length} networks`;
      meta.appendChild(document.createTextNode(
        `${count} · ${(wp.channels || []).length} channels · ${wp.sweepMs || 0}ms sweep · ${new Date(result.time).toLocaleString()}`));
    }
  }

  renderWirelessStations(wp, result);

  const filteredOut = total.length > 0 && allNetworks.length === 0;
  $("#wl-empty").textContent = filteredOut ? "No networks match the filter." : "No networks detected yet.";
  $("#wl-empty").classList.toggle("hidden", allNetworks.length > 0 || (!!result && !filteredOut));
  if (result && result.error) {
    $("#wl-empty").textContent = "Last sweep failed: " + result.error;
    $("#wl-empty").classList.remove("hidden");
  }

  // Refresh or close the detail panel against the new sweep
  const detailNet = wlDetailBssid && allNetworks.find((n) => n.bssid === wlDetailBssid);
  if (detailNet) {
    showApDetail(detailNet, wp, result);
  } else if (wlDetailBssid) {
    wlDetailBssid = null;
    $("#wl-detail").classList.add("hidden");
  }

  if (!allNetworks.length) return;

  // Count clients per BSSID
  const clientsPerBssid = new Map();
  if (wp && wp.stations) {
    for (const s of wp.stations) {
      if (s.bssid) {
        clientsPerBssid.set(s.bssid, (clientsPerBssid.get(s.bssid) || 0) + 1);
      }
    }
  }

  // Group networks by SSID (non-empty only)
  const groupedBySSID = new Map(); // ssid → [networks]
  const hiddenNetworks = [];
  for (const n of allNetworks) {
    if (!n.ssid) {
      hiddenNetworks.push(n);
    } else {
      if (!groupedBySSID.has(n.ssid)) {
        groupedBySSID.set(n.ssid, []);
      }
      groupedBySSID.get(n.ssid).push(n);
    }
  }

  const bandLabel = (freq) => (freq >= 5955 ? "6 GHz" : freq >= 5000 ? "5 GHz" : "2.4 GHz");
  // Entries retained from earlier sweeps (agent keeps them ~10 min) get dimmed
  const resultMs = result ? new Date(result.time).getTime() : 0;
  const isStale = (v) => toMs(v) && resultMs && resultMs - toMs(v) > 2 * 60 * 1000;
  const lastSeenText = (v) =>
    toMs(v) ? new Date(toMs(v)).toLocaleTimeString() : result ? new Date(result.time).toLocaleTimeString() : "";

  // Helper to render a single network row
  function renderNetworkRow(n, isChild = false) {
    const rssiClass = signalClass(n.rssiDbm);
    const clients = clientsPerBssid.get(n.bssid) || 0;

    const tr = document.createElement("tr");
    if (isChild) tr.className = "wl-child-row";
    if (isStale(n.lastSeenMs)) tr.classList.add("wl-stale");
    tr.innerHTML = `
      <td>${isChild ? "&nbsp;&nbsp;" : ""}${n.ssid ? esc(n.ssid) : '<span class="muted">(hidden)</span>'}</td>
      <td class="mono">${esc(n.bssid)}</td>
      <td class="num"><span class="health ${rssiClass}">${n.rssiDbm || 0}</span> ${renderSignalBar(n.rssiDbm)}</td>
      <td class="num">${n.channel || "—"}</td>
      <td>${bandLabel(n.freqMhz || 0)}</td>
      <td>${n.standards || "—"}</td>
      <td>${n.security || "Open"}</td>
      <td class="num">${clients}</td>
      <td class="muted nowrap">${lastSeenText(n.lastSeenMs)}</td>`;
    tr.style.cursor = "pointer";
    tr.addEventListener("click", () => showApDetail(n, wp, result));
    return tr;
  }

  // Render grouped SSIDs
  for (const [ssid, networks] of groupedBySSID.entries()) {
    if (networks.length === 1) {
      // Single AP: render as plain row without chevron
      tbody.appendChild(renderNetworkRow(networks[0]));
    } else {
      // Multiple APs: summary group row; expanding lists every AP underneath
      const strongest = networks[0]; // Already sorted by RSSI
      const channels = [...new Set(networks.map((n) => n.channel).filter(Boolean))].sort((a, b) => a - b);
      const bands = [...new Set(networks.map((n) => bandLabel(n.freqMhz || 0)))];
      const clients = networks.reduce((sum, n) => sum + (clientsPerBssid.get(n.bssid) || 0), 0);
      const newestMs = Math.max(...networks.map((n) => toMs(n.lastSeenMs)));

      const groupTr = document.createElement("tr");
      groupTr.className = "wl-group-row";
      groupTr.innerHTML = `
        <td><span class="chevron">▸</span> ${esc(ssid)} <span class="ap-count">${networks.length} APs</span></td>
        <td class="muted">${networks.length} BSSIDs</td>
        <td class="num"><span class="health ${signalClass(strongest.rssiDbm)}">${strongest.rssiDbm || 0}</span> ${renderSignalBar(strongest.rssiDbm)}</td>
        <td class="num">${channels.join(", ") || "—"}</td>
        <td>${bands.join(" / ")}</td>
        <td>${strongest.standards || "—"}</td>
        <td>${strongest.security || "Open"}</td>
        <td class="num">${clients}</td>
        <td class="muted nowrap">${lastSeenText(newestMs)}</td>`;
      groupTr.style.cursor = "pointer";
      tbody.appendChild(groupTr);

      // All APs of the SSID as child rows, hidden until expanded
      let expanded = false;
      const childRows = networks.map((n) => {
        const childTr = renderNetworkRow(n, true);
        childTr.classList.add("hidden");
        tbody.appendChild(childTr);
        return childTr;
      });

      // Toggle expansion on click
      groupTr.addEventListener("click", () => {
        expanded = !expanded;
        groupTr.querySelector(".chevron").textContent = expanded ? "▾" : "▸";
        for (const childTr of childRows) {
          childTr.classList.toggle("hidden", !expanded);
        }
      });
    }
  }

  // Render hidden networks (not grouped)
  for (const n of hiddenNetworks) {
    tbody.appendChild(renderNetworkRow(n));
  }
}

// renderWirelessStations fills the client-stations table from the last sweep.
function renderWirelessStations(wp, result) {
  const tbody = $("#wl-stations-table tbody");
  tbody.innerHTML = "";
  const stations = ((wp && wp.stations) || []).slice().sort((a, b) => (b.rssiDbm || -999) - (a.rssiDbm || -999));

  // Map BSSID → SSID for the network column
  const ssidByBssid = new Map();
  for (const n of (wp && wp.networks) || []) {
    if (n.ssid) ssidByBssid.set(n.bssid, n.ssid);
  }

  $("#wl-stations-empty").classList.toggle("hidden", stations.length > 0);
  const assoc = stations.filter((s) => !s.probeOnly).length;
  $("#wl-stations-meta").textContent = stations.length
    ? `${stations.length} stations (${assoc} associated, ${stations.length - assoc} probing)`
    : "";
  if (!stations.length) return;

  const resultMs = result ? new Date(result.time).getTime() : 0;
  for (const s of stations) {
    const net = s.probeOnly || !s.bssid
      ? '<span class="muted">probing</span>'
      : esc(s.ssid || ssidByBssid.get(s.bssid) || "") || `<span class="mono">${esc(s.bssid)}</span>`;
    const tr = document.createElement("tr");
    if (toMs(s.lastSeenMs) && resultMs && resultMs - toMs(s.lastSeenMs) > 2 * 60 * 1000) tr.classList.add("wl-stale");
    tr.innerHTML = `
      <td class="mono">${esc(s.mac)}</td>
      <td><span class="wl-vendor muted" data-mac="${esc(s.mac)}"></span></td>
      <td>${net}</td>
      <td class="num"><span class="health ${signalClass(s.rssiDbm)}">${s.rssiDbm || 0}</span> ${renderSignalBar(s.rssiDbm)}</td>
      <td class="num">${s.rateMbps ? s.rateMbps.toFixed(1) : "—"}</td>
      <td class="num">${s.mcs >= 0 ? s.mcs : "—"}</td>
      <td class="num">${s.frames || 0}</td>
      <td class="muted nowrap">${toMs(s.lastSeenMs) ? new Date(toMs(s.lastSeenMs)).toLocaleTimeString() : "—"}</td>`;
    tbody.appendChild(tr);
  }
  fillVendors("#wl-stations-table");
}

// --- Path (traceroute) ---
let paAgents = [];
let paTests = [];
let paHistoryResults = null;  // cached for theme re-render
let paHeatmapInstance = null;  // ECharts instance
let paWaterfallInstance = null;  // ECharts waterfall instance
let paDisplayedResult = null;  // cached result for theme re-render and Back to latest
let paLatestResult = null;  // latest result for "Back to latest" button
let paMetric = "latency";  // toggle between "latency", "jitter", and "loss"

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

// Metric toggle (Latency / Jitter / Loss)
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
    } else if (paMetric === "jitter") {
      waterfallTitle.textContent = "Jitter by hop";
      historyTitle.textContent = "Path history — jitter";
    } else {
      waterfallTitle.textContent = "Round-trip time by hop";
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
  if (currentSection() === "wireless" && wlActiveLast) {
    renderWlActiveWaterfall(wlActiveLast);
  }
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
  const tbody = $("#pa-hop-table tbody");
  statusEl.innerHTML = "";
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

// Render one path result (status, hops table, waterfall)
function renderPathResult(r, agent) {
  paDisplayedResult = r;  // cache for theme re-render

  const t = r.payload && r.payload.traceroute;
  const statusEl = $("#pa-status");
  const tbody = $("#pa-hop-table tbody");

  if (!t) {
    statusEl.innerHTML = '<p class="empty">No path data in this result.</p>';
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

  // Hop table with latency, loss, and jitter inline bars
  const hops = t.hops || [];
  const maxWorst = Math.max(0.1, ...hops.filter((h) => h.host).map((h) => h.worstRttMs || 0));
  const maxJitter = Math.max(0.1, ...hops.filter((h) => h.host).map((h) => h.jitterMs || 0));
  tbody.innerHTML = "";
  for (const h of hops) {
    const tr = document.createElement("tr");
    let host;
    if (h.host) {
      const display = h.hostName || h.host;
      const secondLine = h.hostName ? `<div class="muted" style="font-size: var(--text-xs); font-family: monospace;">${esc(h.host)}</div>` : '';
      host = `<div>${esc(display)}</div>${secondLine}`;
    } else {
      host = '<span class="muted">* * *</span>';
    }
    const latencyCell = h.host ? renderLatencyBarWithValue(h.bestRttMs, h.avgRttMs, h.worstRttMs, maxWorst, h.lossPercent) : '<span class="muted">no reply</span>';
    const lossBarCell = h.host ? renderLossBar(h.lossPercent) : '<span class="muted">–</span>';
    const jitterBarCell = h.host && h.jitterMs ? renderJitterBar(h.jitterMs, maxJitter) : '<span class="muted">–</span>';
    tr.innerHTML = `
      <td class="num">${h.ttl}</td>
      <td>${host}</td>
      <td>${latencyCell}</td>
      <td>${lossBarCell}</td>
      <td>${jitterBarCell}</td>`;
    tbody.appendChild(tr);
  }

  // Waterfall chart
  renderPathWaterfall(hops);
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
        hostName: h.hostName,
        cumulative: currentCumulative,
        delta: delta,
        prevCumulative: prevCumulative,
      });

      prevCumulative = currentCumulative;
    }

    // Prepare data for horizontal stacked bars (waterfall effect)
    const baseData = [];
    const deltaData = [];
    const negativeDeltaTicks = [];

    for (let i = 0; i < waterfallData.length; i++) {
      const d = waterfallData[i];
      // Base: invisible bar to position the visible bar
      baseData.push(Math.min(d.prevCumulative, d.cumulative));
      // Delta: visible portion (0 for negative deltas)
      if (d.delta < 0) {
        deltaData.push(0);
        negativeDeltaTicks.push([d.cumulative, i]);
      } else {
        const barHeight = Math.abs(d.delta);
        deltaData.push(barHeight);
      }
    }

    // Convert scatter data to full array aligned with waterfallData (with nulls for non-negative deltas)
    const scatterDataFull = waterfallData.map((d, i) =>
      d.delta < 0 ? [d.cumulative, i] : null
    );

    const maxCum = Math.max(...waterfallData.map((d) => d.cumulative));
    const maxNice = niceMax(maxCum * 1.1);

    // Truncate host label to ~24 chars
    const labels = waterfallData.map((d) => {
      const display = d.hostName || d.host;
      const label = `${d.ttl}  ${display}`;
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
          const label = d.delta < 0 ? "measured faster than previous hop (independent probe)" : "RTT increase vs previous hop";
          let hostLine = d.hostName ? `<strong>${esc(d.hostName)}</strong> (${esc(d.host)})` : `<strong>${esc(d.host)}</strong>`;
          return `${hostLine} (TTL ${d.ttl})<br/>` +
            `${label}: ${sign}${fmt(d.delta)} ms<br/>` +
            `RTT to this hop: ${fmt(d.cumulative)} ms`;
        }
      },
      grid: { top: 44, bottom: 24, left: 140, right: 30 },
      xAxis: {
        type: "value",
        position: "top",
        min: 0,
        max: maxNice,
        name: "ms",
        nameGap: 6,
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
        },
        {
          name: "negative-delta-tick",
          type: "scatter",
          symbol: "rect",
          symbolSize: [3, 16],
          data: scatterDataFull,
          itemStyle: { color: mutedColor },
          label: { show: false },
        }
      ],
      textStyle: { color: textColor },
    };
  } else if (paMetric === "jitter") {
    // Jitter mode: plain horizontal bars, not cumulative
    const jitterData = respondingHops.map((h) => ({
      ttl: h.ttl,
      host: h.host,
      hostName: h.hostName,
      jitter: h.jitterMs || 0,
      avgRtt: h.avgRttMs || 0,
    }));

    // Truncate host label to ~24 chars
    const labels = jitterData.map((d) => {
      const display = d.hostName || d.host;
      const label = `${d.ttl}  ${display}`;
      return label.length > 24 ? label.substring(0, 21) + "…" : label;
    });

    const maxJitter = Math.max(0.1, ...jitterData.map((d) => d.jitter));
    const maxNice = niceMax(maxJitter * 1.1);

    const jitterValues = jitterData.map((d) => ({
      value: d.jitter,
      itemStyle: {
        color: accentColor,
      }
    }));

    option = {
      tooltip: {
        trigger: "axis",
        axisPointer: { type: "shadow" },
        formatter: function (params) {
          if (!params || !params[0]) return "";
          const dataIndex = params[0].dataIndex;
          const d = jitterData[dataIndex];
          let hostLine = d.hostName ? `<strong>${esc(d.hostName)}</strong> (${esc(d.host)})` : `<strong>${esc(d.host)}</strong>`;
          return `${hostLine} (TTL ${d.ttl})<br/>` +
            `Jitter: ${fmt(d.jitter)} ms<br/>` +
            `Avg RTT: ${fmt(d.avgRtt)} ms`;
        }
      },
      grid: { top: 44, bottom: 24, left: 140, right: 30 },
      xAxis: {
        type: "value",
        position: "top",
        min: 0,
        max: maxNice,
        name: "ms",
        nameGap: 6,
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
          name: "Jitter",
          type: "bar",
          barWidth: 16,
          data: jitterValues,
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
      hostName: h.hostName,
      loss: h.lossPercent || 0,
      avgRtt: h.avgRttMs || 0,
    }));

    // Truncate host label to ~24 chars
    const labels = lossData.map((d) => {
      const display = d.hostName || d.host;
      const label = `${d.ttl}  ${display}`;
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
          let hostLine = d.hostName ? `<strong>${esc(d.hostName)}</strong> (${esc(d.host)})` : `<strong>${esc(d.host)}</strong>`;
          return `${hostLine} (TTL ${d.ttl})<br/>` +
            `Loss: ${fmt(d.loss, 0)}%<br/>` +
            `Avg RTT: ${fmt(d.avgRtt)} ms`;
        }
      },
      grid: { top: 44, bottom: 24, left: 140, right: 30 },
      xAxis: {
        type: "value",
        position: "top",
        min: 0,
        max: 100,
        name: "%",
        nameGap: 6,
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
  const chartHeight = Math.max(180, respondingHops.length * 28 + 100);

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

// Render latency range bar with value text for a hop
function renderLatencyBarWithValue(best, avg, worst, maxWorst, loss) {
  if (!best || !avg || !worst) return "–";

  const bestPct = (best / maxWorst) * 100;
  const worstPct = (worst / maxWorst) * 100;
  const avgPct = (avg / maxWorst) * 100;
  const barColor = loss >= 60 ? "var(--bad)" : loss >= 20 ? "var(--warn)" : "var(--ok)";

  return `<div style="display: flex; align-items: center; gap: 8px;">
    <span style="font-size: var(--text-xs); font-family: var(--mono); font-weight: 550; width: 40px; text-align: right;">${fmt(avg)} ms</span>
    <div class="latency-bar-container">
      <div class="latency-track">
        <div class="latency-range" style="left: ${bestPct}%; width: ${worstPct - bestPct}%; background: ${barColor};"></div>
        <div class="latency-avg" style="left: ${avgPct}%;"></div>
      </div>
    </div>
  </div>`;
}

// Render loss bar for a hop
function renderLossBar(loss) {
  if (loss == null) return "–";
  const lossColor = loss >= 60 ? "var(--bad)" : loss >= 20 ? "var(--warn)" : "var(--ok)";
  const lossPercentValue = (loss / 100) * 100;

  return `<div style="display: flex; align-items: center; gap: 8px;">
    <span style="font-size: var(--text-xs); font-family: var(--mono); font-weight: 550; width: 30px; text-align: right;">${fmt(loss, 0)}%</span>
    <div class="latency-bar-container">
      <div class="latency-track">
        <div style="position: absolute; height: 100%; width: ${lossPercentValue}%; background: ${lossColor}; border-radius: var(--radius-xs);"></div>
      </div>
    </div>
  </div>`;
}

// Render jitter bar for a hop
function renderJitterBar(jitter, maxJitter) {
  if (jitter == null || maxJitter == null) return "–";
  const jitterPercentValue = (jitter / maxJitter) * 100;

  return `<div style="display: flex; align-items: center; gap: 8px;">
    <span style="font-size: var(--text-xs); font-family: var(--mono); font-weight: 550; width: 40px; text-align: right;">${fmt(jitter)} ms</span>
    <div class="latency-bar-container">
      <div class="latency-track">
        <div style="position: absolute; height: 100%; width: ${jitterPercentValue}%; background: var(--accent); border-radius: var(--radius-xs);"></div>
      </div>
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
  let maxJitter = 0;

  for (const r of reversed) {
    const t = r.payload && r.payload.traceroute;
    if (!t || !t.hops) continue;

    for (const h of t.hops) {
      if (h.host) {
        allHops.add(h.ttl);
        let value;
        if (paMetric === "loss") {
          value = h.lossPercent;
        } else if (paMetric === "jitter") {
          value = h.jitterMs || 0;
        } else {
          value = h.avgRttMs;
        }
        heatmapData.push([
          r.time,  // raw r.time as category key
          h.ttl,
          value,
        ]);
        if (paMetric === "latency") {
          maxAvgRtt = Math.max(maxAvgRtt, h.avgRttMs || 0);
        } else if (paMetric === "jitter") {
          maxJitter = Math.max(maxJitter, h.jitterMs || 0);
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
  } else if (paMetric === "jitter") {
    const maxNice = niceMax(Math.max(0.1, maxJitter) * 1.1);
    visualMapConfig = {
      min: 0,
      max: maxNice,
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
            const display = hop.hostName || hop.host;
            const hostStr = hop.hostName ? `${display} (${hop.host})` : display;
            if (paMetric === "loss") {
              hopInfo = `<br/>${hostStr}<br/>Loss: ${fmt(hop.lossPercent, 0)}%<br/>Avg RTT: ${fmt(hop.avgRttMs)} ms`;
            } else if (paMetric === "jitter") {
              hopInfo = `<br/>${hostStr}<br/>Jitter: ${fmt(hop.jitterMs || 0)} ms<br/>Avg RTT: ${fmt(hop.avgRttMs)} ms`;
            } else {
              hopInfo = `<br/>${hostStr}<br/>Best: ${fmt(hop.bestRttMs)} ms<br/>Worst: ${fmt(hop.worstRttMs)} ms<br/>Loss: ${fmt(hop.lossPercent, 0)}%`;
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
        name: paMetric === "loss" ? "Loss (%)" : paMetric === "jitter" ? "Jitter (ms)" : "Avg RTT (ms)",
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

// Metric applicability map: which test types can use which metrics
const METRIC_APPLICABILITY = {
  unhealthy: ["ping", "dns", "http", "tcp", "traceroute", "wlan_passive", "speedtest"],
  latency_ms: ["ping", "dns", "http", "tcp", "traceroute", "speedtest"],
  loss_percent: ["ping"],
  download_mbps: ["speedtest"],
  upload_mbps: ["speedtest"],
};

// Get rules applicable to a test type
function getApplicableRules(testType) {
  if (!testType) return [];
  return alertRules.filter((r) => {
    const applicableMetrics = Object.entries(METRIC_APPLICABILITY)
      .filter(([_, types]) => types.includes(testType))
      .map(([metric, _]) => metric);
    return applicableMetrics.includes(r.metric);
  });
}

function updateAlertBadge(count) {
  const badge = $("#nav-alert-badge");
  badge.textContent = count > 0 ? count : "";
  badge.classList.toggle("hidden", !count);
}

async function loadAlerts() {
  const alerts = await api("GET", "/api/v1/alerts" + tenantParam());

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
}

$("#ar-metric").addEventListener("change", () => {
  const metric = $("#ar-metric").value;
  $("#ar-threshold-wrap").classList.toggle("hidden", metric === "unhealthy" || metric === "state");
  $("#ar-state-wrap").classList.toggle("hidden", metric !== "state");
});

let editingRuleId = null;

$("#btn-new-rule").addEventListener("click", async () => {
  editingRuleId = null;
  const [tests, targets] = await Promise.all([
    api("GET", "/api/v1/tests" + tenantParam()),
    api("GET", "/api/v1/alert-targets" + tenantParam()),
  ]);
  if (!tests.length) { alert("Create a test first."); return; }
  $("#dlg-rule-title").textContent = "New alert rule";
  $("#ar-name").value = "";
  $("#ar-test").innerHTML = tests.map((t) => `<option value="${t.id}">${esc(t.name)} (${t.type})</option>`).join("");
  // Preselect the pending test if available
  if (pendingTestForRule) {
    $("#ar-test").value = pendingTestForRule;
    pendingTestForRule = null;
  }
  $("#ar-metric").value = "unhealthy";
  $("#ar-threshold-wrap").classList.add("hidden");
  $("#ar-state-wrap").classList.add("hidden");
  $("#ar-threshold").value = 0;
  $("#ar-state-level").value = "2";
  $("#ar-forcount").value = 2;
  $("#ar-clearthreshold").value = "";
  $("#ar-clearcount").value = 1;
  // Populate target checkboxes
  const targetsList = $("#ar-targets-list");
  targetsList.innerHTML = targets.map((t) => `
    <label class="check"><input type="checkbox" value="${t.id}" class="ar-target-checkbox"> ${esc(t.name)} (${t.type})</label>
  `).join("");
  dialogError("#ar-error", "");
  $("#dlg-rule").showModal();
});

$("#form-rule").addEventListener("submit", async (e) => {
  e.preventDefault();
  const metric = $("#ar-metric").value;
  const clearThresholdVal = $("#ar-clearthreshold").value.trim();
  const targetIds = Array.from(document.querySelectorAll(".ar-target-checkbox:checked")).map(el => el.value);
  const body = {
    name: $("#ar-name").value.trim(),
    testId: $("#ar-test").value,
    metric,
    operator: metric === "unhealthy" ? ">" : (metric === "state" ? ">=" : $("#ar-op").value),
    threshold: metric === "unhealthy" ? 0 : (metric === "state" ? +$("#ar-state-level").value : +$("#ar-threshold").value),
    forCount: +$("#ar-forcount").value,
    clearThreshold: clearThresholdVal ? +clearThresholdVal : null,
    clearCount: +$("#ar-clearcount").value,
    targetIds,
  };
  const tid = tenantParam("");
  if (tid) body.tenantId = tid.split("=")[1];
  try {
    if (editingRuleId) {
      await api("PUT", `/api/v1/alert-rules/${editingRuleId}`, body);
    } else {
      await api("POST", "/api/v1/alert-rules", body);
    }
    $("#dlg-rule").close();
    loadAlertCfg();
  } catch (err) {
    dialogError("#ar-error", err.message);
  }
});

// --- Alert Configuration ---
async function loadAlertCfg() {
  const [targets, rules] = await Promise.all([
    api("GET", "/api/v1/alert-targets" + tenantParam()),
    api("GET", "/api/v1/alert-rules" + tenantParam()),
  ]);
  // Update the cached alertRules
  alertRules = rules;

  // Render targets table
  const tt = $("#targets-table tbody");
  tt.innerHTML = "";
  // Add built-in Dashboard row
  const dashboardRow = document.createElement("tr");
  dashboardRow.innerHTML = `
    <td><strong>Dashboard</strong></td>
    <td><span class="chip">built-in</span></td>
    <td class="muted">Always visible in Alerts</td>
    <td style="text-align:right">—</td>`;
  tt.appendChild(dashboardRow);
  // Add user targets
  for (const t of targets) {
    const summary = t.type === "webhook" ? esc(t.config.url || "")
      : t.type === "email" ? esc((t.config.to || []).join(", "))
      : t.type === "script" ? esc(t.config.path || "")
      : t.type === "snmp" ? esc((t.config.host || "") + ":" + (t.config.port || 162))
      : "—";
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><strong>${esc(t.name)}</strong></td>
      <td><span class="chip">${t.type}</span></td>
      <td class="muted">${summary}</td>
      <td style="text-align:right"><button class="edit-target" data-id="${t.id}">Edit</button> <button class="danger" data-del="${t.id}">Delete</button></td>`;
    tr.querySelector(".edit-target").addEventListener("click", () => editAlertTarget(t));
    tr.querySelector("[data-del]").addEventListener("click", async () => {
      if (!confirm(`Delete target "${t.name}"?`)) return;
      await api("DELETE", `/api/v1/alert-targets/${t.id}`);
      loadAlertCfg();
    });
    tt.appendChild(tr);
  }
  $("#targets-empty").classList.toggle("hidden", targets.length > 0);

  // Render rules table
  const rt = $("#rules-table tbody");
  rt.innerHTML = "";
  for (const r of rules) {
    const cond = r.metric === "unhealthy"
      ? "is unhealthy"
      : `${METRIC_LABEL[r.metric] || r.metric} ${r.operator} ${r.threshold}`;
    const clearCond = r.metric === "unhealthy"
      ? "passes"
      : r.clearThreshold !== null && r.clearThreshold !== undefined
      ? `${METRIC_LABEL[r.metric] || r.metric} ${r.operator === ">" ? "<" : r.operator === ">=" ? "<" : r.operator === "<" ? ">" : r.operator === "<=" ? ">" : "!="} ${r.clearThreshold}`
      : "no longer breaching";
    const targetNames = (r.targetIds || []).length > 0
      ? r.targetIds.map(id => {
          const t = targets.find(t => t.id === id);
          return t ? t.name : id;
        }).join(", ")
      : "—";
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><strong>${esc(r.name)}</strong></td>
      <td>${esc(r.testName)}</td>
      <td class="muted">${esc(cond)} ×${r.forCount}</td>
      <td class="muted">${esc(clearCond)} ×${r.clearCount}</td>
      <td class="muted">${esc(targetNames)}</td>
      <td style="text-align:right"><button class="edit-rule" data-id="${r.id}">Edit</button> <button class="danger" data-del="${r.id}">Delete</button></td>`;
    tr.querySelector(".edit-rule").addEventListener("click", () => editAlertRule(r, targets));
    tr.querySelector("[data-del]").addEventListener("click", async () => {
      if (!confirm(`Delete rule "${r.name}"?`)) return;
      await api("DELETE", `/api/v1/alert-rules/${r.id}`);
      loadAlertCfg();
    });
    rt.appendChild(tr);
  }
  $("#rules-empty").classList.toggle("hidden", rules.length > 0);

  // Open the new rule dialog if pendingTestForRule is set
  if (pendingTestForRule) {
    // Trigger the "New rule" button click to open the dialog
    $("#btn-new-rule").click();
  }
}

async function editAlertRule(rule, targets) {
  editingRuleId = rule.id;
  const tests = await api("GET", "/api/v1/tests" + tenantParam());
  $("#dlg-rule-title").textContent = "Edit alert rule";
  $("#ar-name").value = rule.name;
  $("#ar-test").innerHTML = tests.map((t) => `<option value="${t.id}" ${t.id === rule.testId ? "selected" : ""}>${esc(t.name)} (${t.type})</option>`).join("");
  $("#ar-metric").value = rule.metric;
  $("#ar-threshold-wrap").classList.toggle("hidden", rule.metric === "unhealthy" || rule.metric === "state");
  $("#ar-state-wrap").classList.toggle("hidden", rule.metric !== "state");
  if (rule.metric !== "unhealthy" && rule.metric !== "state") {
    $("#ar-op").value = rule.operator;
    $("#ar-threshold").value = rule.threshold;
  }
  if (rule.metric === "state") {
    $("#ar-state-level").value = rule.threshold;
  }
  $("#ar-forcount").value = rule.forCount;
  $("#ar-clearthreshold").value = rule.clearThreshold || "";
  $("#ar-clearcount").value = rule.clearCount || 1;
  // Populate target checkboxes
  const targetsList = $("#ar-targets-list");
  targetsList.innerHTML = targets.map((t) => `
    <label class="check"><input type="checkbox" value="${t.id}" class="ar-target-checkbox" ${(rule.targetIds || []).includes(t.id) ? "checked" : ""}> ${esc(t.name)} (${t.type})</label>
  `).join("");
  dialogError("#ar-error", "");
  $("#dlg-rule").showModal();
}

let editingTargetId = null;

$("#btn-new-target").addEventListener("click", () => {
  editingTargetId = null;
  $("#dlg-target-title").textContent = "New alert target";
  $("#at-name").value = "";
  $("#at-type").value = "webhook";
  $("#at-webhook-url").value = "";
  $("#at-email-to").value = "";
  $("#at-email-subject").value = "";
  $("#at-snmp-host").value = "";
  $("#at-snmp-port").value = "162";
  $("#at-snmp-community").value = "public";
  $("#at-script-path").value = "";
  $("#at-script-args").value = "";
  // Hide script option for non-admins
  $("#at-type-script").classList.toggle("hidden", !me.isAdmin);
  showTargetTypeFields("webhook");
  dialogError("#at-error", "");
  $("#dlg-target").showModal();
});

function editAlertTarget(target) {
  editingTargetId = target.id;
  $("#dlg-target-title").textContent = "Edit alert target";
  $("#at-name").value = target.name;
  $("#at-type").value = target.type;
  if (target.type === "webhook") {
    $("#at-webhook-url").value = target.config.url || "";
  } else if (target.type === "email") {
    $("#at-email-to").value = (target.config.to || []).join(", ");
    $("#at-email-subject").value = target.config.subject || "";
  } else if (target.type === "snmp") {
    $("#at-snmp-host").value = target.config.host || "";
    $("#at-snmp-port").value = target.config.port || 162;
    $("#at-snmp-community").value = target.config.community || "public";
  } else if (target.type === "script") {
    $("#at-script-path").value = target.config.path || "";
    $("#at-script-args").value = JSON.stringify(target.config.args || []);
  }
  // Hide script option for non-admins
  $("#at-type-script").classList.toggle("hidden", !me.isAdmin);
  showTargetTypeFields(target.type);
  dialogError("#at-error", "");
  $("#dlg-target").showModal();
}

$("#at-type").addEventListener("change", () => {
  showTargetTypeFields($("#at-type").value);
});

function showTargetTypeFields(type) {
  $("#at-webhook-params").classList.toggle("hidden", type !== "webhook");
  $("#at-email-params").classList.toggle("hidden", type !== "email");
  $("#at-snmp-params").classList.toggle("hidden", type !== "snmp");
  $("#at-script-params").classList.toggle("hidden", type !== "script");
}

$("#form-target").addEventListener("submit", async (e) => {
  e.preventDefault();
  const type = $("#at-type").value;
  const config = {};

  if (type === "webhook") {
    config.url = $("#at-webhook-url").value.trim();
  } else if (type === "email") {
    const to = $("#at-email-to").value.trim();
    config.to = to.split(",").map(s => s.trim()).filter(s => s);
    const subject = $("#at-email-subject").value.trim();
    if (subject) config.subject = subject;
  } else if (type === "snmp") {
    config.host = $("#at-snmp-host").value.trim();
    config.port = +$("#at-snmp-port").value;
    config.community = $("#at-snmp-community").value.trim();
  } else if (type === "script") {
    config.path = $("#at-script-path").value.trim();
    const argsStr = $("#at-script-args").value.trim();
    if (argsStr) {
      try {
        config.args = JSON.parse(argsStr);
      } catch (err) {
        dialogError("#at-error", "Invalid JSON for args");
        return;
      }
    }
  }

  const body = {
    name: $("#at-name").value.trim(),
    type,
    config,
  };
  const tid = tenantParam("");
  if (tid) body.tenantId = tid.split("=")[1];

  try {
    if (editingTargetId) {
      await api("PUT", `/api/v1/alert-targets/${editingTargetId}`, body);
    } else {
      await api("POST", "/api/v1/alert-targets", body);
    }
    $("#dlg-target").close();
    loadAlertCfg();
  } catch (err) {
    dialogError("#at-error", err.message);
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
