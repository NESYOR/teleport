/*
 * Teleport
 * Copyright (C) 2026 Gravitational, Inc.
 */

package livemap

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>Teleport Auth Event Stream -> Live Topology Builder</title>
<style>
  :root {
    --bg: #0b0f19;
    --panel: #131b2e;
    --border: #23304d;
    --text: #e2e8f0;
    --muted: #94a3b8;
    --auth: #a855f7;
    --proxy: #38bdf8;
    --node: #22c55e;
    --user: #f59e0b;
    --danger: #ef4444;
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    background: var(--bg);
    color: var(--text);
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, monospace;
    display: flex;
    flex-direction: column;
    height: 100vh;
    overflow: hidden;
  }
  header {
    padding: 12px 20px;
    background: var(--panel);
    border-bottom: 1px solid var(--border);
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  header h1 { font-size: 16px; font-weight: 600; letter-spacing: 0.4px; }
  .controls { display: flex; gap: 8px; flex-wrap: wrap; }
  button {
    background: #1e293b;
    color: var(--text);
    border: 1px solid #3b82f6;
    padding: 6px 12px;
    border-radius: 6px;
    cursor: pointer;
    font-size: 12px;
    transition: all 0.15s;
  }
  button:hover { background: #2563eb; }
  button.danger { border-color: var(--danger); }
  button.danger:hover { background: #dc2626; }
  main {
    display: grid;
    grid-template-columns: 1.3fr 1fr;
    flex: 1;
    overflow: hidden;
  }
  .canvas-wrap {
    position: relative;
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
  }
  canvas { flex: 1; width: 100%; height: 100%; display: block; }
  .legend {
    position: absolute;
    bottom: 12px;
    left: 12px;
    background: rgba(19, 27, 46, 0.9);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 10px 14px;
    font-size: 11px;
    display: flex;
    gap: 14px;
    flex-wrap: wrap;
  }
  .dot { display: inline-block; width: 9px; height: 9px; border-radius: 50%; margin-right: 5px; }
  .sidebar {
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .section-title {
    padding: 8px 14px;
    background: #182238;
    border-bottom: 1px solid var(--border);
    font-size: 12px;
    font-weight: 600;
    color: #93c5fd;
    display: flex;
    justify-content: space-between;
  }
  .edges-box {
    max-height: 210px;
    overflow-y: auto;
    border-bottom: 1px solid var(--border);
  }
  .events-box {
    flex: 1;
    overflow-y: auto;
  }
  table { width: 100%; border-collapse: collapse; font-size: 11.5px; }
  th, td {
    padding: 6px 10px;
    text-align: left;
    border-bottom: 1px solid rgba(35, 48, 77, 0.5);
  }
  th { color: var(--muted); font-weight: 500; position: sticky; top: 0; background: var(--panel); }
  .badge {
    padding: 2px 6px;
    border-radius: 4px;
    font-size: 10px;
    font-weight: 600;
  }
  .b-HEARTBEAT, .b-CONTROL_STREAM, .b-INSTANCE_HB { background: rgba(34, 197, 94, 0.2); color: #4ade80; }
  .b-TUNNEL_HB { background: rgba(56, 189, 248, 0.25); color: #38bdf8; }
  .b-LIVE_SESSION { background: rgba(245, 158, 11, 0.25); color: #fbbf24; }
  .b-ACCESS_REQUEST { background: rgba(16, 185, 129, 0.25); color: #34d399; }
  .b-SESSION_LOCK { background: rgba(239, 68, 68, 0.3); color: #f87171; }
  .b-CA_ROTATION { background: rgba(168, 85, 247, 0.25); color: #c084fc; }
  .b-RBAC_CHANGE, .b-CONFIG_CHANGE { background: rgba(99, 102, 241, 0.25); color: #818cf8; }
  .b-AUDIT_EVENT { background: rgba(236, 72, 153, 0.25); color: #f472b6; }
  .b-WATCH_EVENT { background: rgba(56, 189, 248, 0.2); color: #38bdf8; }
  .b-GRPC_UNARY, .b-GRPC_STREAM { background: rgba(168, 85, 247, 0.2); color: #c084fc; }
  .b-AUTHORIZER { background: rgba(245, 158, 11, 0.2); color: #fbbf24; }
  .b-RBAC_CHECK { background: rgba(59, 130, 246, 0.2); color: #60a5fa; }
  .b-DENIED, .b-RESOURCE_EXPIRED { background: rgba(239, 68, 68, 0.25); color: #f87171; }
  #status { font-size: 12px; color: #4ade80; }
</style>
</head>
<body>
<header>
  <div>
    <h1>Teleport Auth & Proxy Event Consumer ──► Dynamic Cluster Topology Map</h1>
    <span id="status">Listening to All 83 Auth & Proxy Watcher Kinds + Audit Stream...</span>
  </div>
  <div class="controls" id="trigger-controls">
    <button onclick="trigger('heartbeat')">Pulse Agent Heartbeat</button>
    <button onclick="trigger('tunnel')">Upsert Reverse Tunnel</button>
    <button onclick="trigger('session')">Start Live Session</button>
    <button onclick="trigger('access_request')">Create AccessRequest</button>
    <button class="danger" onclick="trigger('lock')">Create Session Lock</button>
    <button onclick="trigger('audit')">Emit Audit Event</button>
    <button class="danger" onclick="trigger('expire_node')">Expire Node (OpDelete)</button>
  </div>
</header>
<main>
  <div class="canvas-wrap">
    <canvas id="map"></canvas>
    <div class="legend">
      <span><i class="dot" style="background:#4ade80"></i>HEARTBEAT / INSTANCE_HB</span>
      <span><i class="dot" style="background:#38bdf8"></i>TUNNEL_HB (Proxy/Cluster)</span>
      <span><i class="dot" style="background:#fbbf24"></i>LIVE_SESSION / WEB_SESSION</span>
      <span><i class="dot" style="background:#34d399"></i>ACCESS_REQUEST</span>
      <span><i class="dot" style="background:#ef4444"></i>SESSION_LOCK / EXPIRED</span>
      <span><i class="dot" style="background:#c084fc"></i>CA_ROTATION / RBAC / CONFIG</span>
      <span><i class="dot" style="background:#f472b6"></i>AUDIT_EVENT</span>
    </div>
  </div>
  <div class="sidebar">
    <div class="section-title">
      <span>DISCOVERED TOPOLOGY EDGES (FROM AUTH & PROXY WATCHERS)</span>
      <span id="edge-count">0 edges</span>
    </div>
    <div class="edges-box">
      <table>
        <thead>
          <tr><th>From ──► To</th><th>Events</th><th>By Type</th><th>Last Observed Event</th></tr>
        </thead>
        <tbody id="edges-tbody"></tbody>
      </table>
    </div>
    <div class="section-title">
      <span>REAL-TIME AUTH & PROXY WATCHER STREAM</span>
      <span id="event-count">0 events</span>
    </div>
    <div class="events-box">
      <table>
        <thead>
          <tr><th>#</th><th>Event Type</th><th>From ──► To</th><th>Resource / Event</th><th>Metadata / Payload</th></tr>
        </thead>
        <tbody id="events-tbody"></tbody>
      </table>
    </div>
  </div>
</main>
<script>
const canvas = document.getElementById('map');
const ctx = canvas.getContext('2d');
let nodes = {};
let edges = {};
let particles = [];
let eventsList = [];

function resize() {
  canvas.width = canvas.clientWidth * window.devicePixelRatio;
  canvas.height = canvas.clientHeight * window.devicePixelRatio;
  ctx.scale(window.devicePixelRatio, window.devicePixelRatio);
}
window.addEventListener('resize', resize);
resize();

function colorForType(ev) {
  if (ev.type === 'SESSION_LOCK' || ev.allowed === false || ev.type === 'RESOURCE_EXPIRED') return '#ef4444';
  switch (ev.type) {
    case 'HEARTBEAT':
    case 'INSTANCE_HB':
    case 'CONTROL_STREAM': return '#4ade80';
    case 'TUNNEL_HB':
    case 'WATCH_EVENT': return '#38bdf8';
    case 'LIVE_SESSION': return '#fbbf24';
    case 'ACCESS_REQUEST': return '#34d399';
    case 'CA_ROTATION':
    case 'RBAC_CHANGE':
    case 'CONFIG_CHANGE':
    case 'GRPC_UNARY':
    case 'GRPC_STREAM': return '#c084fc';
    case 'AUDIT_EVENT': return '#f472b6';
    default: return '#60a5fa';
  }
}

function colorForNodeKind(kind) {
  switch (kind) {
    case 'auth': return '#a855f7';
    case 'proxy': return '#38bdf8';
    case 'node':
    case 'db':
    case 'kube':
    case 'app': return '#22c55e';
    default: return '#f59e0b';
  }
}

function layoutNodes() {
  const w = canvas.clientWidth;
  const h = canvas.clientHeight;
  const cx = w / 2;
  const cy = h / 2;
  const keys = Object.keys(nodes);
  const authKey = keys.find(k => nodes[k].kind === 'auth');
  const others = keys.filter(k => k !== authKey);

  if (authKey && nodes[authKey]) {
    nodes[authKey].x = cx;
    nodes[authKey].y = cy;
  }
  const radius = Math.min(w, h) * 0.35;
  others.forEach((k, idx) => {
    const angle = (idx / Math.max(1, others.length)) * Math.PI * 2 - Math.PI / 2;
    nodes[k].x = cx + Math.cos(angle) * radius;
    nodes[k].y = cy + Math.sin(angle) * radius;
  });
}

function recordIncomingEvent(ev) {
  if (ev.type === 'RESOURCE_EXPIRED' && ev.from && ev.from.startsWith('node:')) {
    delete nodes[ev.from];
    Object.keys(edges).forEach(k => {
      if (edges[k].from === ev.from || edges[k].to === ev.from) delete edges[k];
    });
    layoutNodes();
    eventsList.unshift(ev);
    renderTables();
    return;
  }

  if (!nodes[ev.from]) nodes[ev.from] = { id: ev.from, label: ev.from_label || ev.from, kind: ev.from_kind || 'user', addr: ev.from_addr || '' };
  if (!nodes[ev.to]) nodes[ev.to] = { id: ev.to, label: ev.to_label || ev.to, kind: ev.to_kind || 'auth' };
  layoutNodes();

  const edgeKey = ev.from + '->' + ev.to;
  if (!edges[edgeKey]) {
    edges[edgeKey] = { from: ev.from, to: ev.to, total_count: 0, by_type: {}, last_event: '' };
  }
  edges[edgeKey].total_count++;
  edges[edgeKey].by_type[ev.type] = (edges[edgeKey].by_type[ev.type] || 0) + 1;
  edges[edgeKey].last_event = ev.summary;

  const fromN = nodes[ev.from];
  const toN = nodes[ev.to];
  if (fromN && toN && ev.from !== ev.to) {
    particles.push({
      x0: fromN.x, y0: fromN.y,
      x1: toN.x, y1: toN.y,
      t: 0,
      color: colorForType(ev),
      label: ev.type + ': ' + ev.summary
    });
  }

  eventsList.unshift(ev);
  if (eventsList.length > 80) eventsList.pop();
  renderTables();
}

function renderTables() {
  const edgeKeys = Object.keys(edges);
  document.getElementById('edge-count').textContent = edgeKeys.length + ' edges';
  const etbody = document.getElementById('edges-tbody');
  etbody.innerHTML = edgeKeys.map(k => {
    const e = edges[k];
    const types = Object.entries(e.by_type).map(([t, c]) => t + ':' + c).join(', ');
    return '<tr><td><b>' + e.from + '</b> ──► <b>' + e.to + '</b></td><td>' + e.total_count + '</td><td>' + types + '</td><td>' + e.last_event + '</td></tr>';
  }).join('');

  document.getElementById('event-count').textContent = eventsList.length + ' recent';
  const vtbody = document.getElementById('events-tbody');
  vtbody.innerHTML = eventsList.slice(0, 40).map(ev => {
    const badgeClass = ev.allowed === false ? 'b-DENIED' : ('b-' + ev.type);
    const label = ev.allowed === false ? (ev.type + ' (DENIED)') : ev.type;
    return '<tr><td>' + ev.seq + '</td><td><span class="badge ' + badgeClass + '">' + label + '</span></td><td>' + ev.from + ' ──► ' + ev.to + '</td><td>' + ev.summary + '</td><td>' + (ev.details || '') + '</td></tr>';
  }).join('');
}

function draw() {
  const w = canvas.clientWidth;
  const h = canvas.clientHeight;
  ctx.clearRect(0, 0, w, h);

  // Draw edges
  Object.values(edges).forEach(e => {
    const n0 = nodes[e.from];
    const n1 = nodes[e.to];
    if (!n0 || !n1 || e.from === e.to) return;
    ctx.beginPath();
    ctx.moveTo(n0.x, n0.y);
    ctx.lineTo(n1.x, n1.y);
    ctx.strokeStyle = 'rgba(148, 163, 184, 0.25)';
    ctx.lineWidth = Math.min(5, 1 + Math.log2(1 + e.total_count));
    ctx.stroke();
  });

  // Draw animated packets
  for (let i = particles.length - 1; i >= 0; i--) {
    const p = particles[i];
    p.t += 0.02;
    if (p.t >= 1) {
      particles.splice(i, 1);
      continue;
    }
    const x = p.x0 + (p.x1 - p.x0) * p.t;
    const y = p.y0 + (p.y1 - p.y0) * p.t;
    ctx.beginPath();
    ctx.arc(x, y, 5, 0, Math.PI * 2);
    ctx.fillStyle = p.color;
    ctx.fill();
    ctx.fillStyle = '#e2e8f0';
    ctx.font = '10px monospace';
    ctx.fillText(p.label, x + 8, y + 3);
  }

  // Draw nodes
  Object.values(nodes).forEach(n => {
    ctx.beginPath();
    ctx.arc(n.x, n.y, n.kind === 'auth' ? 28 : 20, 0, Math.PI * 2);
    ctx.fillStyle = '#131b2e';
    ctx.fill();
    ctx.lineWidth = 3;
    ctx.strokeStyle = colorForNodeKind(n.kind);
    ctx.stroke();

    ctx.fillStyle = '#f8fafc';
    ctx.font = 'bold 11px monospace';
    ctx.textAlign = 'center';
    ctx.fillText(n.label || n.id, n.x, n.y + 4);
    ctx.fillStyle = '#94a3b8';
    ctx.font = '10px monospace';
    ctx.fillText('[' + (n.kind || 'node').toUpperCase() + '] ' + (n.addr || ''), n.x, n.y + 34);
  });

  requestAnimationFrame(draw);
}
requestAnimationFrame(draw);

async function init() {
  const res = await fetch('/snapshot');
  const snap = await res.json();
  (snap.nodes || []).forEach(n => { nodes[n.id] = n; });
  layoutNodes();
  (snap.edges || []).forEach(e => { edges[e.from + '->' + e.to] = e; });
  eventsList = (snap.recent_events || []).reverse();
  renderTables();

  const es = new EventSource('/events');
  es.onopen = () => { document.getElementById('status').textContent = 'Connected (Consuming Live Auth Watcher & Audit Stream)'; };
  es.onmessage = (msg) => {
    const ev = JSON.parse(msg.data);
    recordIncomingEvent(ev);
  };
}

async function trigger(action) {
  const statusEl = document.getElementById('status');
  statusEl.textContent = 'Dispatching Auth event: ' + action + '...';
  const res = await fetch('/trigger?action=' + encodeURIComponent(action), { method: 'POST' });
  const data = await res.json();
  statusEl.textContent = data.ok ? ('Emitted: ' + data.message) : ('Error: ' + data.error);
}

init();
</script>
</body>
</html>
`
