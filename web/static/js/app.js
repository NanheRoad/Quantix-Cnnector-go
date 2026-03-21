const API_KEY = localStorage.getItem('quantix_api_key') || 'quantix-dev-key';
const BASE = `${location.protocol}//${location.host}`;
document.getElementById('backend-url').textContent = BASE;

const state = {
  activeTab: 'dashboard',
  dashboardStore: {},
  health: null,
  ws: null,
  protocols: [],
  devices: [],
  controlManualSteps: [],
  serialSeq: 0,
  serialLogs: [],
  protocolEditors: {
    new: { syncingFromTemplate: false, syncingFromFields: false, lastAnalysis: null, highlightLine: null, lineCount: 0 },
    edit: { syncingFromTemplate: false, syncingFromFields: false, lastAnalysis: null, highlightLine: null, lineCount: 0 },
  },
};

const STATUS_TEXT = {
  online: '在线',
  offline: '离线',
  error: '异常',
};

const PROTOCOL_TYPE_TEXT = {
  modbus_tcp: 'Modbus TCP',
  modbus_rtu: 'Modbus RTU',
  mqtt: 'MQTT',
  serial: '串口',
  tcp: 'TCP',
};

const PROTOCOL_DESC_TEXT = {
  'Standard Modbus TCP scale template': '标准 Modbus TCP 称重模板',
  'MQTT push weight data': 'MQTT 推送称重数据模板',
  'TSC serial print template': 'TSC 串口打印模板',
  'TSC tcp print template': 'TSC TCP 打印模板',
  'Serial scanner line mode': '串口扫码枪行模式模板',
  'Serial board polling template': '串口看板轮询模板',
};

const ALLOWED_PROTOCOL_TYPES = new Set(['modbus_tcp', 'modbus_rtu', 'mqtt', 'serial', 'tcp', 'modbus']);
const ALLOWED_TRIGGERS = new Set(['poll', 'manual', 'setup', 'event']);
const ALLOWED_PARSE_TYPES = new Set(['expression', 'regex', 'substring', 'struct']);

function el(id) { return document.getElementById(id); }
function pretty(v) { return JSON.stringify(v, null, 2); }
function pad2(v) { return String(v).padStart(2, '0'); }
function pad3(v) { return String(v).padStart(3, '0'); }
function formatTimestamp(raw) {
  if (!raw) return '-';
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return String(raw);
  const y = d.getFullYear();
  const m = pad2(d.getMonth() + 1);
  const day = pad2(d.getDate());
  const hh = pad2(d.getHours());
  const mm = pad2(d.getMinutes());
  const ss = pad2(d.getSeconds());
  const ms = pad3(d.getMilliseconds());
  return `${y}年${m}月${day}日 ${hh}时${mm}分${ss}秒${ms}毫秒`;
}
function safeParseJSON(text, fallback) {
  try {
    const v = JSON.parse(text || '');
    return v;
  } catch {
    return fallback;
  }
}

function normalizeTemplateVarDefault(def) {
  if (def === null || def === undefined) return '';
  return def;
}

function buildTemplateDefaultVars(template) {
  const vars = {};
  const list = Array.isArray(template?.variables) ? template.variables : [];
  list.forEach(item => {
    if (!item || typeof item !== 'object' || Array.isArray(item)) return;
    const name = String(item.name || '').trim();
    if (!name) return;
    vars[name] = normalizeTemplateVarDefault(item.default);
  });
  return vars;
}

function mergeTemplateVars(defaultVars, currentVars) {
  const merged = { ...defaultVars };
  if (!currentVars || typeof currentVars !== 'object' || Array.isArray(currentVars)) return merged;
  Object.keys(currentVars).forEach(k => {
    if (Object.prototype.hasOwnProperty.call(defaultVars, k)) {
      merged[k] = currentVars[k];
    }
  });
  return merged;
}

async function fetchProtocolTemplateByID(protocolID) {
  const id = Number(protocolID || 0);
  if (!id) return null;
  const inList = state.protocols.find(p => Number(p.id) === id);
  if (inList && inList.template && typeof inList.template === 'object') return inList.template;
  const detail = await api(`/api/protocols/${id}`);
  return detail?.template || null;
}

async function applyDeviceTemplateDefaults(kind) {
  const selectID = kind === 'new' ? 'new-device-protocol' : 'edit-device-protocol';
  const varsID = kind === 'new' ? 'new-device-vars' : 'edit-device-vars';
  const resultID = kind === 'new' ? 'create-device-result' : 'update-device-result';
  const protocolID = Number(el(selectID).value || 0);
  if (!protocolID) return;
  try {
    const template = await fetchProtocolTemplateByID(protocolID);
    const defaults = buildTemplateDefaultVars(template);
    const current = safeParseJSON(el(varsID).value, {});
    const merged = mergeTemplateVars(defaults, current);
    el(varsID).value = pretty(merged);
    el(resultID).textContent = '';
  } catch (e) {
    el(resultID).className = 'err';
    el(resultID).textContent = `加载模板默认变量失败: ${e.message}`;
  }
}

function parseJSONOrThrow(text, fieldName) {
  try {
    return JSON.parse(text || '');
  } catch (e) {
    throw new Error(`${fieldName} 不是合法 JSON: ${e.message}`);
  }
}

function formatWeightValue(value, decimals) {
  const n = Number(value);
  if (!Number.isFinite(n)) return value ?? '--';
  if (decimals == null) return `${n}`;
  const d = Number(decimals);
  if (!Number.isFinite(d)) return `${n}`;
  const fixed = Math.max(0, Math.min(6, Math.trunc(d)));
  return n.toFixed(fixed);
}

function resolveWeightDecimals(rt) {
  if (!rt || typeof rt !== 'object') return null;
  const direct = rt.decimals;
  if (Number.isFinite(Number(direct))) return Number(direct);
  const payload = rt.payload || {};
  if (Number.isFinite(Number(payload.decimals))) return Number(payload.decimals);
  return null;
}

function protocolFieldRefs(kind) {
  return {
    name: el(`${kind}-protocol-name`),
    desc: el(`${kind}-protocol-desc`),
    type: el(`${kind}-protocol-type`),
    template: el(`${kind}-protocol-template`),
    syntax: el(`${kind}-protocol-template-syntax`),
    gutter: el(`${kind}-protocol-template-gutter`),
    status: el(`${kind}-protocol-template-status`),
    errors: el(`${kind}-protocol-template-errors`),
  };
}

function escapeHTML(text) {
  return String(text)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;');
}

function highlightJSON(text) {
  const src = String(text ?? '');
  if (!src) return ' ';
  const tokenRe = /("(?:\\u[\da-fA-F]{4}|\\[^u]|[^\\"])*"(\s*:)?|\btrue\b|\bfalse\b|\bnull\b|-?\d+(?:\.\d+)?(?:[eE][+\-]?\d+)?|[{}\[\],:])/g;
  let out = '';
  let last = 0;
  let m;
  while ((m = tokenRe.exec(src)) !== null) {
    const idx = m.index;
    if (idx > last) out += escapeHTML(src.slice(last, idx));
    const token = m[0];
    let cls = 'json-token-invalid';
    if (token[0] === '"') cls = m[2] ? 'json-token-key' : 'json-token-string';
    else if (token === 'true' || token === 'false') cls = 'json-token-bool';
    else if (token === 'null') cls = 'json-token-null';
    else if (/^-?\d/.test(token)) cls = 'json-token-number';
    else cls = 'json-token-punc';
    out += `<span class="${cls}">${escapeHTML(token)}</span>`;
    last = idx + token.length;
  }
  if (last < src.length) out += escapeHTML(src.slice(last));
  return out;
}

function posToLineCol(text, pos) {
  const safePos = Math.max(0, Math.min(Number(pos) || 0, text.length));
  const lines = text.slice(0, safePos).split('\n');
  return { line: lines.length, col: lines[lines.length - 1].length + 1 };
}

function lineColToPos(text, line, col) {
  const rows = text.split('\n');
  const targetLine = Math.max(1, Math.min(Number(line) || 1, rows.length));
  const targetCol = Math.max(1, Number(col) || 1);
  let idx = 0;
  for (let i = 1; i < targetLine; i += 1) idx += rows[i - 1].length + 1;
  idx += Math.min(targetCol - 1, rows[targetLine - 1].length);
  return idx;
}

function focusEditorAt(textarea, line, col) {
  const pos = lineColToPos(textarea.value, line, col);
  textarea.focus();
  textarea.setSelectionRange(pos, pos);
}

function parseJSONDetailed(text) {
  try {
    return { ok: true, value: JSON.parse(text) };
  } catch (e) {
    const msg = String(e?.message || 'JSON 语法错误');
    let line = 1;
    let col = 1;
    let index = -1;
    let m = msg.match(/position\s+(\d+)/i);
    if (m) {
      index = Number(m[1]);
      const lc = posToLineCol(text, index);
      line = lc.line;
      col = lc.col;
    } else {
      m = msg.match(/line\s+(\d+)\s+column\s+(\d+)/i);
      if (m) {
        line = Number(m[1]);
        col = Number(m[2]);
      }
    }
    return { ok: false, error: { message: msg, line, col, index } };
  }
}

function validateProtocolTemplate(template) {
  const diagnostics = [];
  if (!template || typeof template !== 'object' || Array.isArray(template)) {
    diagnostics.push({ level: 'error', message: '模板 JSON 顶层必须是对象', path: '$' });
    return diagnostics;
  }
  const protocolType = String(template.protocol_type || '').trim().toLowerCase();
  if (!protocolType) {
    diagnostics.push({ level: 'error', message: '缺少 protocol_type', path: '$.protocol_type' });
  } else if (!ALLOWED_PROTOCOL_TYPES.has(protocolType)) {
    diagnostics.push({ level: 'error', message: `protocol_type 不合法: ${protocolType}`, path: '$.protocol_type' });
  }
  const steps = Array.isArray(template.steps) ? template.steps : [];
  const setupSteps = Array.isArray(template.setup_steps) ? template.setup_steps : [];
  const messageHandler = template.message_handler;
  if (steps.length === 0 && setupSteps.length === 0 && !messageHandler) {
    diagnostics.push({ level: 'warn', message: '未配置 steps/setup_steps/message_handler，模板不会执行任何动作', path: '$' });
  }
  const idSet = new Set();
  steps.forEach((step, i) => {
    const path = `$.steps[${i}]`;
    if (!step || typeof step !== 'object' || Array.isArray(step)) {
      diagnostics.push({ level: 'error', message: '步骤必须是对象', path });
      return;
    }
    const stepID = String(step.id || '').trim();
    if (!stepID) {
      diagnostics.push({ level: 'error', message: '步骤缺少 id', path: `${path}.id` });
    } else if (idSet.has(stepID)) {
      diagnostics.push({ level: 'error', message: `步骤 id 重复: ${stepID}`, path: `${path}.id` });
    } else {
      idSet.add(stepID);
    }
    const trigger = String(step.trigger || 'poll').trim().toLowerCase();
    if (!ALLOWED_TRIGGERS.has(trigger)) {
      diagnostics.push({ level: 'error', message: `trigger 不合法: ${trigger}`, path: `${path}.trigger` });
    }
    const action = String(step.action || '').trim();
    if (!action) {
      diagnostics.push({ level: 'error', message: '步骤缺少 action', path: `${path}.action` });
    }
    if (step.parse && typeof step.parse === 'object' && !Array.isArray(step.parse)) {
      const parseType = String(step.parse.type || '').trim().toLowerCase();
      if (!parseType || !ALLOWED_PARSE_TYPES.has(parseType)) {
        diagnostics.push({ level: 'error', message: `parse.type 不支持: ${parseType || '-'}`, path: `${path}.parse.type` });
      }
    }
    if (/^(modbus\.write|mqtt\.publish|serial\.send|tcp\.send)/.test(action) && trigger !== 'manual') {
      diagnostics.push({ level: 'warn', message: `写操作建议使用 manual trigger: ${action}`, path: `${path}.trigger` });
    }
  });
  if (protocolType === 'mqtt' && (!messageHandler || typeof messageHandler !== 'object')) {
    diagnostics.push({ level: 'warn', message: 'MQTT 模板建议配置 message_handler 处理推送数据', path: '$.message_handler' });
  }
  return diagnostics;
}

function tryLocatePath(text, path) {
  if (!path || path === '$') return null;
  const keyMatch = path.match(/\.([a-zA-Z0-9_]+)$/);
  if (!keyMatch) return null;
  const needle = `"${keyMatch[1]}"`;
  const idx = text.indexOf(needle);
  if (idx < 0) return null;
  return posToLineCol(text, idx);
}

function getEditorLineHeight(textarea) {
  const lh = parseFloat(getComputedStyle(textarea).lineHeight);
  return Number.isFinite(lh) && lh > 0 ? lh : 19;
}

function renderProtocolGutter(kind, highlightLine) {
  const refs = protocolFieldRefs(kind);
  const editorState = state.protocolEditors[kind];
  const lineCount = Math.max(1, refs.template.value.split('\n').length);
  if (editorState.lineCount !== lineCount) {
    const nums = [];
    for (let i = 1; i <= lineCount; i += 1) {
      nums.push(`<div class="json-gutter-line${highlightLine === i ? ' err' : ''}" data-line="${i}">${i}</div>`);
    }
    refs.gutter.innerHTML = nums.join('');
    editorState.lineCount = lineCount;
  } else {
    refs.gutter.querySelectorAll('.json-gutter-line').forEach(node => {
      node.classList.toggle('err', Number(node.dataset.line) === highlightLine);
    });
  }
  refs.gutter.style.transform = `translateY(${-refs.template.scrollTop}px)`;
}

function renderProtocolSyntax(kind) {
  const refs = protocolFieldRefs(kind);
  if (!refs.syntax) return;
  refs.syntax.innerHTML = highlightJSON(refs.template.value || '');
  refs.syntax.style.transform = `translateY(${-refs.template.scrollTop}px)`;
}

function renderProtocolTextareaHighlight(kind, highlightLine) {
  const refs = protocolFieldRefs(kind);
  const ta = refs.template;
  if (!highlightLine) {
    ta.style.backgroundImage = 'none';
    return;
  }
  const styles = getComputedStyle(ta);
  const lineHeight = getEditorLineHeight(ta);
  const padTop = parseFloat(styles.paddingTop) || 0;
  const top = padTop + (highlightLine - 1) * lineHeight - ta.scrollTop;
  const bottom = top + lineHeight;
  if (bottom < 0 || top > ta.clientHeight) {
    ta.style.backgroundImage = 'none';
    return;
  }
  const topPx = Math.max(0, top);
  const bottomPx = Math.min(ta.clientHeight, bottom);
  ta.style.backgroundImage = `linear-gradient(to bottom,
    transparent 0,
    transparent ${topPx}px,
    rgba(180, 35, 44, 0.14) ${topPx}px,
    rgba(180, 35, 44, 0.14) ${bottomPx}px,
    transparent ${bottomPx}px
  )`;
  ta.style.backgroundRepeat = 'no-repeat';
}

function renderProtocolEditorVisual(kind, highlightLine) {
  renderProtocolSyntax(kind);
  renderProtocolGutter(kind, highlightLine);
  renderProtocolTextareaHighlight(kind, highlightLine);
}

function renderProtocolDiagnostics(kind, analysis) {
  const refs = protocolFieldRefs(kind);
  const editorState = state.protocolEditors[kind];
  refs.errors.innerHTML = '';
  refs.status.className = 'editor-status';

  const actionBtn = kind === 'new' ? el('create-protocol-btn') : el('update-protocol-btn');
  let hasHardError = false;
  let highlightLine = null;

  if (!analysis.ok) {
    hasHardError = true;
    highlightLine = analysis.error.line || 1;
    refs.status.classList.add('err');
    refs.status.textContent = `JSON 语法错误（第 ${analysis.error.line} 行，第 ${analysis.error.col} 列）：${analysis.error.message}`;
    const li = document.createElement('li');
    li.textContent = `语法错误 @ ${analysis.error.line}:${analysis.error.col}`;
    li.addEventListener('click', () => focusEditorAt(refs.template, analysis.error.line, analysis.error.col));
    refs.errors.appendChild(li);
  } else {
    const diags = analysis.diagnostics || [];
    const hardCount = diags.filter(d => d.level === 'error').length;
    const warnCount = diags.filter(d => d.level === 'warn').length;
    hasHardError = hardCount > 0;
    if (hardCount > 0) {
      refs.status.classList.add('err');
      refs.status.textContent = `JSON 诊断：${hardCount} 个错误，${warnCount} 个警告`;
    } else {
      refs.status.classList.add('ok');
      refs.status.textContent = `JSON 诊断：通过（${warnCount} 个警告）`;
    }
    diags.forEach(d => {
      const li = document.createElement('li');
      if (d.level === 'warn') li.className = 'warn';
      const located = tryLocatePath(refs.template.value, d.path);
      if (located) {
        if (!highlightLine && d.level === 'error') highlightLine = located.line;
        li.textContent = `${d.level === 'error' ? '错误' : '警告'} ${d.path} @ ${located.line}:${located.col} - ${d.message}`;
        li.addEventListener('click', () => focusEditorAt(refs.template, located.line, located.col));
      } else {
        li.textContent = `${d.level === 'error' ? '错误' : '警告'} ${d.path} - ${d.message}`;
      }
      refs.errors.appendChild(li);
    });
  }
  editorState.highlightLine = highlightLine;
  actionBtn.disabled = hasHardError;
  renderProtocolEditorVisual(kind, highlightLine);
}

function analyzeProtocolEditor(kind) {
  const refs = protocolFieldRefs(kind);
  const parsed = parseJSONDetailed(refs.template.value || '{}');
  if (!parsed.ok) return parsed;
  if (!parsed.value || typeof parsed.value !== 'object' || Array.isArray(parsed.value)) {
    return { ok: true, value: parsed.value, diagnostics: [{ level: 'error', message: '模板 JSON 顶层必须是对象', path: '$' }] };
  }
  return { ok: true, value: parsed.value, diagnostics: validateProtocolTemplate(parsed.value) };
}

function syncProtocolFieldsFromTemplate(kind, template) {
  const editorState = state.protocolEditors[kind];
  if (editorState.syncingFromFields) return;
  const refs = protocolFieldRefs(kind);
  editorState.syncingFromTemplate = true;
  refs.name.value = String(template?.name || '');
  refs.desc.value = String(template?.description || '');
  const templateType = String(template?.protocol_type || '').trim();
  if (kind === 'new') {
    if ([...refs.type.options].some(o => o.value === templateType)) refs.type.value = templateType;
  } else {
    refs.type.value = templateType;
  }
  editorState.syncingFromTemplate = false;
}

function onProtocolTemplateInput(kind) {
  const editorState = state.protocolEditors[kind];
  if (editorState.syncingFromFields) return;
  const analysis = analyzeProtocolEditor(kind);
  editorState.lastAnalysis = analysis;
  if (analysis.ok && analysis.value && typeof analysis.value === 'object' && !Array.isArray(analysis.value)) {
    syncProtocolFieldsFromTemplate(kind, analysis.value);
  }
  renderProtocolDiagnostics(kind, analysis);
}

function syncProtocolTemplateFromFields(kind) {
  const editorState = state.protocolEditors[kind];
  if (editorState.syncingFromTemplate) return;
  const refs = protocolFieldRefs(kind);
  const parsed = parseJSONDetailed(refs.template.value || '{}');
  const obj = parsed.ok && parsed.value && typeof parsed.value === 'object' && !Array.isArray(parsed.value) ? parsed.value : {};
  editorState.syncingFromFields = true;
  obj.name = refs.name.value;
  obj.description = refs.desc.value;
  obj.protocol_type = refs.type.value;
  refs.template.value = pretty(obj);
  editorState.syncingFromFields = false;
  onProtocolTemplateInput(kind);
}

function formatProtocolTemplate(kind) {
  const refs = protocolFieldRefs(kind);
  const parsed = parseJSONDetailed(refs.template.value || '{}');
  if (!parsed.ok) {
    renderProtocolDiagnostics(kind, parsed);
    return;
  }
  refs.template.value = pretty(parsed.value);
  onProtocolTemplateInput(kind);
}

function getProtocolPayload(kind) {
  const refs = protocolFieldRefs(kind);
  syncProtocolTemplateFromFields(kind);
  const analysis = analyzeProtocolEditor(kind);
  renderProtocolDiagnostics(kind, analysis);
  if (!analysis.ok) {
    throw new Error(`模板 JSON 语法错误（第 ${analysis.error.line} 行，第 ${analysis.error.col} 列）`);
  }
  const hard = (analysis.diagnostics || []).find(d => d.level === 'error');
  if (hard) throw new Error(`模板校验失败：${hard.message}`);
  return {
    name: refs.name.value,
    description: refs.desc.value,
    protocol_type: refs.type.value,
    template: analysis.value,
  };
}

function initProtocolEditors() {
  syncProtocolTemplateFromFields('new');
  renderProtocolEditorVisual('new', state.protocolEditors.new.highlightLine);
  el('edit-protocol-template').value = '{}';
  renderProtocolEditorVisual('edit', null);
  el('edit-protocol-template-status').className = 'editor-status';
  el('edit-protocol-template-status').textContent = 'JSON 诊断：请选择协议';
  el('edit-protocol-template-errors').innerHTML = '';
  el('update-protocol-btn').disabled = true;
}

function protocolTypeText(type) {
  const key = String(type || '').trim().toLowerCase();
  return PROTOCOL_TYPE_TEXT[key] || type || '-';
}

function protocolDescText(desc) {
  const text = String(desc || '').trim();
  if (!text) return '-';
  return PROTOCOL_DESC_TEXT[text] || text;
}

async function api(path, options = {}) {
  const headers = { 'X-API-Key': API_KEY, ...(options.headers || {}) };
  const resp = await fetch(`${BASE}${path}`, { ...options, headers });
  const text = await resp.text();
  let data = {};
  try { data = text ? JSON.parse(text) : {}; } catch { data = { raw: text }; }
  if (!resp.ok) {
    const detail = data?.detail || data?.error || `${resp.status}`;
    throw new Error(detail);
  }
  return data;
}

function setupTabs() {
  document.querySelectorAll('.tab-btn').forEach(btn => {
    btn.addEventListener('click', () => switchTab(btn.dataset.tab));
  });
}

function switchTab(tab) {
  state.activeTab = tab;
  document.querySelectorAll('.tab-btn').forEach(btn => btn.classList.toggle('active', btn.dataset.tab === tab));
  document.querySelectorAll('.page').forEach(page => page.classList.toggle('active', page.id === `page-${tab}`));
  if (tab === 'dashboard') openDashboardWS();
  else closeDashboardWS();
  refreshCurrentTab();
}

function statusBadge(status) {
  const cls = status === 'online' ? 'online' : status === 'error' ? 'error' : 'offline';
  return `<span class="badge ${cls}">${STATUS_TEXT[status] || status || '未知'}</span>`;
}

function mergeDashboard(payload) {
  const id = payload.device_id;
  if (!id) return;
  const existing = state.dashboardStore[id] || {};
  state.dashboardStore[id] = {
    ...existing,
    runtime: { ...(existing.runtime || {}), ...payload },
    id,
    name: payload.device_name || existing.name || `#${id}`,
    device_code: payload.device_code || existing.device_code,
    device_category: payload.device_category || existing.device_category || 'weight',
  };
}

function renderDashboard() {
  const container = el('dashboard-cards');
  const cards = Object.values(state.dashboardStore)
    .sort((a, b) => (a.id || 0) - (b.id || 0))
    .map(item => {
      const rt = item.runtime || {};
      const payload = rt.payload || {};
      let metric = `${formatWeightValue(rt.weight, resolveWeightDecimals(rt))} ${rt.unit || 'kg'}`;
      let extra = '-';
      if (item.device_category === 'printer_tsc') {
        metric = `打印回执: ${payload.print_ack ?? '--'}`;
        extra = `任务ID: ${payload.job_id ?? '-'}`;
      } else if (item.device_category === 'scanner') {
        metric = `条码: ${payload.barcode ?? '--'}`;
        extra = `码制: ${payload.symbology ?? '-'} | 去重: ${payload.deduped ? '是' : '否'}`;
      } else if (item.device_category === 'serial_board') {
        metric = `数值: ${payload.board_value ?? '--'}`;
        extra = `状态: ${payload.board_status ?? '-'} | 告警: ${payload.alarm ? '是' : '否'}`;
      }
      return `<div class="card">
        <h4>${item.name || '-'}</h4>
        <small>编码: ${item.device_code || '-'} | 分类: ${item.device_category || 'weight'}</small>
        <div style="font-size:30px;margin:8px 0">${metric}</div>
        <div>${statusBadge(rt.status || 'offline')}</div>
        <small>${extra}</small>
        <div><small>更新时间: ${formatTimestamp(rt.timestamp)}</small></div>
        <div class="err"><small>${rt.error || ''}</small></div>
      </div>`;
    });
  container.innerHTML = cards.join('');
}

function renderDashboardMetrics() {
  const box = el('dashboard-metrics');
  const health = state.health || {};
  const metrics = health.metrics || {};
  const bus = metrics.event_bus_stats || {};
  const status = health.status || 'ok';
  box.innerHTML = [
    `健康状态: <b>${status === 'ok' ? '正常' : status === 'degraded' ? '降级' : status}</b>`,
    `运行设备: ${health.online_count || 0}/${health.runtime_count || 0} 在线`,
    `轮询延迟 P95: ${Number(metrics.latency_p95_ms || 0).toFixed(2)} ms`,
    `轮询延迟 P99: ${Number(metrics.latency_p99_ms || 0).toFixed(2)} ms`,
    `轮询错误: ${metrics.poll_errors || 0}`,
    `重连次数: ${metrics.reconnects || 0}`,
    `运行时重启: ${metrics.runtime_restarts || 0}`,
    `事件丢弃: ${bus.dropped || 0}`,
  ].join(' | ');
}

async function refreshDashboardMetrics() {
  try {
    state.health = await api('/health');
    renderDashboardMetrics();
  } catch (e) {
    el('dashboard-metrics').innerHTML = `<span class="err">读取健康指标失败: ${e.message}</span>`;
  }
}

async function refreshDashboardFallback() {
  try {
    const devices = await api('/api/devices');
    state.dashboardStore = {};
    for (const d of devices) state.dashboardStore[d.id] = d;
    renderDashboard();
    await refreshDashboardMetrics();
    el('ws-status').textContent = '实时通道: WebSocket（每 10 秒兜底同步）';
    el('dashboard-error').textContent = '';
  } catch (e) {
    el('dashboard-error').textContent = `加载失败: ${e.message}`;
  }
}

function openDashboardWS() {
  if (state.ws) return;
  const url = `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws?api_key=${encodeURIComponent(API_KEY)}`;
  const ws = new WebSocket(url);
  state.ws = ws;
  ws.onopen = () => { el('ws-status').textContent = 'WebSocket 已连接'; };
  ws.onclose = () => {
    state.ws = null;
    el('ws-status').textContent = 'WebSocket 重连中...';
    if (state.activeTab === 'dashboard') setTimeout(openDashboardWS, 1000);
  };
  ws.onmessage = ev => {
    const msg = safeParseJSON(ev.data, null);
    if (!msg) return;
    if (msg.type === 'ping') {
      el('ws-status').textContent = 'WebSocket 心跳正常';
      return;
    }
    if (!['weight_update', 'print_event', 'scan_event', 'board_event'].includes(msg.type)) return;
    mergeDashboard(msg);
    renderDashboard();
  };
}

function closeDashboardWS() {
  if (state.ws) {
    state.ws.close();
    state.ws = null;
  }
}

async function refreshDevices() {
  if (state.activeTab !== 'devices') return;
  try {
    const devices = await api('/api/devices');
    state.devices = devices;
    const rows = devices.map(d => {
      const rt = d.runtime || {};
      const displayData = rt.weight != null
        ? formatWeightValue(rt.weight, resolveWeightDecimals(rt))
        : (rt.barcode ?? rt.board_value ?? '-');
      return `<tr>
        <td>${d.id}</td><td>${d.device_code || '-'}</td><td>${d.name}</td><td>${d.device_category || 'weight'}</td>
        <td>${d.protocol_template_id}</td><td>${STATUS_TEXT[rt.status] || rt.status || '离线'}</td>
        <td>${displayData}</td><td>${formatTimestamp(rt.timestamp)}</td><td>${rt.error || '-'}</td>
        <td>${d.enabled ? '是' : '否'}</td>
        <td><button data-edit-device="${d.id}" class="btn-sec">编辑</button> <button data-delete-device="${d.id}">删除</button></td>
      </tr>`;
    }).join('');
    el('devices-table').innerHTML = `<table><thead><tr>
      <th>ID</th><th>编码</th><th>名称</th><th>分类</th><th>模板</th><th>状态</th><th>数据</th><th>时间</th><th>错误</th><th>启用</th><th>操作</th>
    </tr></thead><tbody>${rows}</tbody></table>`;
    el('devices-error').textContent = '';
    refreshDeviceEditOptions();
    el('devices-table').querySelectorAll('button[data-edit-device]').forEach(btn => {
      btn.addEventListener('click', () => openDeviceEditor(btn.dataset.editDevice));
    });
    el('devices-table').querySelectorAll('button[data-delete-device]').forEach(btn => {
      btn.addEventListener('click', () => deleteDevice(btn.dataset.deleteDevice));
    });
  } catch (e) {
    el('devices-error').textContent = `加载失败: ${e.message}`;
  }
}

async function loadProtocolOptions() {
  try {
    state.protocols = await api('/api/protocols');
  } catch {
    state.protocols = [];
  }
  const html = state.protocols.map(p => `<option value="${p.id}">#${p.id} ${p.name} (${p.protocol_type})</option>`).join('');
  el('new-device-protocol').innerHTML = html;
  el('edit-device-protocol').innerHTML = html;
  el('edit-protocol-id').innerHTML = `<option value="">请选择协议</option>${html}`;
  el('step-test-protocol').innerHTML = html;
  if (!el('new-device-protocol').value && state.protocols.length > 0) {
    el('new-device-protocol').value = String(state.protocols[0].id);
  }
  await applyDeviceTemplateDefaults('new');
}

function refreshDeviceEditOptions() {
  const select = el('edit-device-id');
  const prev = String(select.value || '');
  const options = state.devices.map(d => `<option value="${d.id}">#${d.id} [${d.device_code || '-'}] ${d.name || '-'}</option>`).join('');
  select.innerHTML = `<option value="">请选择设备</option>${options}`;
  if (prev && state.devices.some(d => String(d.id) === prev)) {
    select.value = prev;
  }
}

function fillDeviceEditForm(device) {
  el('edit-device-id').value = String(device.id || '');
  el('edit-device-name').value = device.name || '';
  el('edit-device-code').value = device.device_code || '';
  el('edit-device-category').value = device.device_category || 'weight';
  el('edit-device-protocol').value = String(device.protocol_template_id || '');
  el('edit-device-poll').value = Number(device.poll_interval || 1);
  el('edit-device-enabled').value = device.enabled ? 'true' : 'false';
  el('edit-device-conn').value = pretty(device.connection_params || {});
  el('edit-device-vars').value = pretty(device.template_variables || {});
}

async function loadEditDevice() {
  const id = Number(el('edit-device-id').value || 0);
  if (!id) return;
  try {
    const d = await api(`/api/devices/${id}`);
    fillDeviceEditForm(d);
    el('update-device-result').textContent = '';
  } catch (e) {
    el('update-device-result').className = 'err';
    el('update-device-result').textContent = `加载设备失败: ${e.message}`;
  }
}

async function openDeviceEditor(id) {
  el('edit-device-id').value = String(id || '');
  await loadEditDevice();
}

async function updateDevice() {
  const id = Number(el('edit-device-id').value || 0);
  if (!id) {
    el('update-device-result').className = 'err';
    el('update-device-result').textContent = '请先选择设备';
    return;
  }
  const payload = {
    device_code: el('edit-device-code').value,
    device_category: el('edit-device-category').value,
    name: el('edit-device-name').value,
    protocol_template_id: Number(el('edit-device-protocol').value),
    connection_params: safeParseJSON(el('edit-device-conn').value, {}),
    template_variables: safeParseJSON(el('edit-device-vars').value, {}),
    poll_interval: Number(el('edit-device-poll').value || 1),
    enabled: el('edit-device-enabled').value === 'true',
  };
  try {
    const updated = await api(`/api/devices/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    el('update-device-result').className = 'ok';
    el('update-device-result').textContent = `保存成功：设备ID=${updated.id}`;
    await refreshDevices();
    await refreshControlDevices();
    await refreshDebugDevices();
    await loadEditDevice();
  } catch (e) {
    el('update-device-result').className = 'err';
    el('update-device-result').textContent = `保存失败: ${e.message}`;
  }
}

async function testDeviceConnection() {
  try {
    const protocolTemplateID = Number(el('edit-device-protocol').value || 0);
    if (!protocolTemplateID) {
      throw new Error('请先选择协议模板');
    }
    const connectionParams = parseJSONOrThrow(el('edit-device-conn').value, '连接参数');
    const result = await api('/api/devices/test-connection', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        protocol_template_id: protocolTemplateID,
        connection_params: connectionParams,
        timeout_ms: 1500,
      }),
    });
    el('test-device-connection-result').className = 'ok';
    el('test-device-connection-result').textContent =
      `连接成功：${result.endpoint}（耗时 ${Number(result.elapsed_ms || 0).toFixed(2)} ms）`;
  } catch (e) {
    el('test-device-connection-result').className = 'err';
    el('test-device-connection-result').textContent = `连接失败: ${e.message}`;
  }
}

async function createDevice() {
  const payload = {
    device_code: el('new-device-code').value,
    device_category: el('new-device-category').value,
    name: el('new-device-name').value,
    protocol_template_id: Number(el('new-device-protocol').value),
    connection_params: safeParseJSON(el('new-device-conn').value, {}),
    template_variables: safeParseJSON(el('new-device-vars').value, {}),
    poll_interval: Number(el('new-device-poll').value || 1),
    enabled: el('new-device-enabled').value === 'true',
  };
  try {
    const created = await api('/api/devices', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) });
    el('create-device-result').className = 'ok';
    el('create-device-result').textContent = `创建成功：设备ID=${created.id}，编码=${created.device_code}`;
    await refreshDevices();
    await refreshControlDevices();
    await refreshDebugDevices();
  } catch (e) {
    el('create-device-result').className = 'err';
    el('create-device-result').textContent = `创建失败: ${e.message}`;
  }
}

async function deleteDevice(id) {
  if (!confirm(`确认删除设备 ${id} 吗？`)) return;
  try {
    await api(`/api/devices/${id}`, { method: 'DELETE' });
    await refreshDevices();
    await refreshControlDevices();
    await refreshDebugDevices();
  } catch (e) {
    el('devices-error').textContent = `删除失败: ${e.message}`;
  }
}

function extractManualSteps(template) {
  const steps = Array.isArray(template?.steps) ? template.steps : [];
  return steps.filter(s => (s.trigger || 'poll') === 'manual' && s.id).map(s => ({ id: s.id, name: s.name || s.id, action: s.action || '' }));
}

function findQuickStepId(manualSteps, command) {
  const keywords = command === 'tare' ? ['tare', '去皮'] : command === 'zero' ? ['zero', '清零', '置零', '归零'] : [];
  for (const step of manualSteps) {
    const text = `${step.id} ${step.name} ${step.action}`.toLowerCase();
    if (keywords.some(k => text.includes(k))) return step.id;
  }
  return null;
}

async function refreshControlDevices() {
  if (state.activeTab !== 'control') return;
  try {
    const devices = await api('/api/devices');
    const enabled = devices.filter(d => d.enabled);
    el('control-device').innerHTML = enabled.map(d => `<option value="${d.id}">#${d.id} [${d.device_code || '-'}] ${d.name}</option>`).join('');
    await loadControlSteps();
  } catch (e) {
    el('control-result').className = 'err';
    el('control-result').textContent = `加载设备失败: ${e.message}`;
  }
}

async function loadControlSteps() {
  const deviceId = Number(el('control-device').value || 0);
  if (!deviceId) {
    el('control-step').innerHTML = '';
    return;
  }
  try {
    const device = await api(`/api/devices/${deviceId}`);
    const protocol = await api(`/api/protocols/${device.protocol_template_id}`);
    state.controlManualSteps = extractManualSteps(protocol.template || {});
    el('control-step').innerHTML = state.controlManualSteps.map(s => `<option value="${s.id}">${s.name} (${s.id}) [${s.action}]</option>`).join('');
  } catch (e) {
    el('control-result').className = 'err';
    el('control-result').textContent = `加载步骤失败: ${e.message}`;
  }
}

async function runControl(mode) {
  const deviceId = Number(el('control-device').value || 0);
  if (!deviceId) return;
  let stepId = el('control-step').value;
  if (mode === 'tare') stepId = findQuickStepId(state.controlManualSteps, 'tare');
  if (mode === 'zero') stepId = findQuickStepId(state.controlManualSteps, 'zero');
  if (!stepId) {
    el('control-result').className = 'err';
    el('control-result').textContent = '未找到匹配的手动步骤';
    return;
  }
  const params = safeParseJSON(el('control-params').value, {});
  try {
    const result = await api(`/api/devices/${deviceId}/execute`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ step_id: stepId, params }),
    });
    el('control-result').className = 'ok';
    const modeText = mode === 'tare' ? '去皮' : mode === 'zero' ? '清零' : '手动';
    el('control-result').textContent = `执行成功：${modeText}（${stepId}）`;
    el('control-detail').textContent = pretty(result);
  } catch (e) {
    el('control-result').className = 'err';
    el('control-result').textContent = `执行失败: ${e.message}`;
  }
}

async function refreshDebugDevices() {
  if (state.activeTab !== 'device-debug') return;
  try {
    const devices = await api('/api/devices');
    const filtered = devices.filter(d => ['printer_tsc', 'scanner', 'serial_board'].includes(d.device_category || 'weight'));
    el('debug-device').innerHTML = filtered.map(d => `<option value="${d.id}">#${d.id} ${d.name} [${d.device_category}]</option>`).join('');
    await refreshDebugRuntime();
  } catch (e) {
    el('debug-result').className = 'err';
    el('debug-result').textContent = `加载失败: ${e.message}`;
  }
}

async function refreshDebugRuntime() {
  if (state.activeTab !== 'device-debug') return;
  const deviceId = Number(el('debug-device').value || 0);
  if (!deviceId) return;
  try {
    const device = await api(`/api/devices/${deviceId}`);
    el('debug-runtime').textContent = pretty({
      device_id: device.id,
      device_code: device.device_code,
      device_category: device.device_category,
      runtime: device.runtime,
    });
    const category = device.device_category || 'weight';
    if (category === 'printer_tsc') {
      el('debug-action').innerHTML = '<option value="print">打印</option>';
    } else if (category === 'scanner') {
      el('debug-action').innerHTML = '<option value="scanner_last">扫码结果</option>';
    } else if (category === 'serial_board') {
      el('debug-action').innerHTML = '<option value="board_status">看板状态</option>';
    }
  } catch (e) {
    el('debug-runtime').textContent = pretty({ 错误: e.message });
  }
}

async function runDebugAction() {
  const deviceId = Number(el('debug-device').value || 0);
  if (!deviceId) return;
  const action = el('debug-action').value;
  const params = safeParseJSON(el('debug-params').value, {});
  try {
    let result;
    if (action === 'print') {
      result = await api(`/api/printers/${deviceId}/print`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ params }),
      });
    } else if (action === 'scanner_last') {
      result = await api(`/api/scanners/${deviceId}/last`);
    } else if (action === 'board_status') {
      result = await api(`/api/boards/${deviceId}/status`);
    }
    el('debug-result').className = 'ok';
    const actionText = action === 'print' ? '打印' : action === 'scanner_last' ? '扫码结果' : '看板状态';
    el('debug-result').textContent = `执行成功：${actionText}`;
    el('debug-action-result').textContent = pretty(result);
    await refreshDebugRuntime();
  } catch (e) {
    el('debug-result').className = 'err';
    el('debug-result').textContent = `执行失败: ${e.message}`;
  }
}

async function refreshProtocols() {
  if (state.activeTab !== 'protocols') return;
  try {
    const protocols = await api('/api/protocols');
    const devices = await api('/api/devices');
    state.protocols = protocols;
    const usage = {};
    for (const d of devices) usage[d.protocol_template_id] = (usage[d.protocol_template_id] || 0) + 1;
    const rows = protocols.map(p => `<tr><td>${p.id}</td><td>${p.name}</td><td>${protocolTypeText(p.protocol_type)}</td><td>${protocolDescText(p.description)}</td><td>${usage[p.id] || 0}</td><td>${p.is_system ? '是' : '否'}</td></tr>`).join('');
    el('protocols-list').innerHTML = `<table><thead><tr><th>ID</th><th>名称</th><th>类型</th><th>描述</th><th>绑定数</th><th>系统模板</th></tr></thead><tbody>${rows}</tbody></table>`;
    const options = protocols.map(p => `<option value="${p.id}">#${p.id} ${p.name} (${p.protocol_type})</option>`).join('');
    el('edit-protocol-id').innerHTML = `<option value="">请选择协议</option>${options}`;
    el('step-test-protocol').innerHTML = options;
    el('protocols-error').textContent = '';
  } catch (e) {
    el('protocols-error').textContent = `加载失败: ${e.message}`;
  }
}

async function createProtocol() {
  try {
    const payload = { ...getProtocolPayload('new'), is_system: false };
    const created = await api('/api/protocols', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) });
    el('create-protocol-result').className = 'ok';
    el('create-protocol-result').textContent = `创建成功：协议ID=${created.id}`;
    await refreshProtocols();
    await loadProtocolOptions();
  } catch (e) {
    el('create-protocol-result').className = 'err';
    el('create-protocol-result').textContent = `创建失败: ${e.message}`;
  }
}

async function loadEditProtocol() {
  const id = Number(el('edit-protocol-id').value || 0);
  if (!id) {
    el('edit-protocol-template').value = '{}';
    state.protocolEditors.edit.highlightLine = null;
    state.protocolEditors.edit.lineCount = 0;
    renderProtocolEditorVisual('edit', null);
    el('edit-protocol-template-status').className = 'editor-status';
    el('edit-protocol-template-status').textContent = 'JSON 诊断：请选择协议';
    el('edit-protocol-template-errors').innerHTML = '';
    el('update-protocol-btn').disabled = true;
    return;
  }
  try {
    const p = await api(`/api/protocols/${id}`);
    el('edit-protocol-name').value = p.name || '';
    el('edit-protocol-desc').value = p.description || '';
    el('edit-protocol-type').value = p.protocol_type || '';
    const editorTemplate = {
      ...(p.template || {}),
      name: p.name || '',
      description: p.description || '',
      protocol_type: p.protocol_type || '',
    };
    el('edit-protocol-template').value = pretty(editorTemplate);
    onProtocolTemplateInput('edit');
  } catch (e) {
    el('edit-protocol-result').className = 'err';
    el('edit-protocol-result').textContent = e.message;
  }
}

async function updateProtocol() {
  const id = Number(el('edit-protocol-id').value || 0);
  if (!id) return;
  try {
    const payload = getProtocolPayload('edit');
    const updated = await api(`/api/protocols/${id}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) });
    el('edit-protocol-result').className = 'ok';
    el('edit-protocol-result').textContent = `更新成功：协议ID=${updated.id}`;
    await refreshProtocols();
    await loadProtocolOptions();
  } catch (e) {
    el('edit-protocol-result').className = 'err';
    el('edit-protocol-result').textContent = `更新失败: ${e.message}`;
  }
}

async function deleteProtocol() {
  const id = Number(el('edit-protocol-id').value || 0);
  if (!id) return;
  if (!confirm(`确认删除协议 ${id} 吗？`)) return;
  try {
    await api(`/api/protocols/${id}`, { method: 'DELETE' });
    el('edit-protocol-result').className = 'ok';
    el('edit-protocol-result').textContent = `删除成功：协议ID=${id}`;
    await refreshProtocols();
    await loadProtocolOptions();
  } catch (e) {
    el('edit-protocol-result').className = 'err';
    el('edit-protocol-result').textContent = `删除失败: ${e.message}`;
  }
}

async function runStepTest() {
  const protocolId = Number(el('step-test-protocol').value || 0);
  if (!protocolId) return;
  const payload = {
    connection_params: safeParseJSON(el('step-test-conn').value, {}),
    template_variables: safeParseJSON(el('step-test-vars').value, {}),
    step_id: el('step-test-id').value,
    step_context: el('step-test-context').value,
    allow_write: el('step-test-allow-write').checked,
    test_payload: el('step-test-payload').value,
    previous_steps: {},
  };
  try {
    const result = await api(`/api/protocols/${protocolId}/test-step`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload),
    });
    el('step-test-result').textContent = pretty(result);
  } catch (e) {
    el('step-test-result').textContent = pretty({ 错误: e.message });
  }
}

async function refreshSerialPorts() {
  try {
    const result = await api('/api/serial-debug/ports');
    const ports = result.ports || [];
    el('serial-port').innerHTML = ports.map(p => `<option value="${p.device}">${p.device} - ${p.description || p.name || ''}</option>`).join('');
  } catch (e) {
    el('serial-status').innerHTML = `<span class="err">端口扫描失败: ${e.message}</span>`;
  }
}

async function serialOpen() {
  const payload = {
    port: el('serial-port').value,
    baudrate: Number(el('serial-baud').value || 9600),
    bytesize: Number(el('serial-bytesize').value || 8),
    parity: el('serial-parity').value,
    stopbits: Number(el('serial-stopbits').value || 1),
    timeout_ms: Number(el('serial-timeout').value || 300),
  };
  try {
    const result = await api('/api/serial-debug/open', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) });
    el('serial-status').innerHTML = `<span class="ok">已连接: ${result.settings?.port || payload.port}</span>`;
  } catch (e) {
    el('serial-status').innerHTML = `<span class="err">打开失败: ${e.message}</span>`;
  }
}

async function serialClose() {
  try {
    await api('/api/serial-debug/close', { method: 'POST' });
    el('serial-status').innerHTML = '<span>已断开</span>';
  } catch (e) {
    el('serial-status').innerHTML = `<span class="err">关闭失败: ${e.message}</span>`;
  }
}

async function serialSend() {
  const payload = {
    data: el('serial-send-data').value,
    data_format: el('serial-data-format').value,
    encoding: el('serial-encoding').value,
    line_ending: el('serial-line-ending').value,
  };
  try {
    const result = await api('/api/serial-debug/send', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) });
    el('serial-send-result').className = 'ok';
    el('serial-send-result').textContent = `发送成功：${result.bytes_sent} 字节`;
  } catch (e) {
    el('serial-send-result').className = 'err';
    el('serial-send-result').textContent = `发送失败: ${e.message}`;
  }
}

async function refreshSerialRuntime() {
  if (state.activeTab !== 'serial-debug') return;
  try {
    const status = await api('/api/serial-debug/status');
    el('serial-status').innerHTML = `<span class="${status.connected ? 'ok' : ''}">${status.connected ? '已连接' : '未连接'}</span> ${status.last_error ? `<span class="err">${status.last_error}</span>` : ''}`;
    if (status.connected) {
      try { await api('/api/serial-debug/read?max_bytes=2048&timeout_ms=30&encoding=utf-8'); } catch {}
    }
    const logs = await api(`/api/serial-debug/logs?last_seq=${state.serialSeq}&limit=400`);
    state.serialSeq = logs.next_seq || state.serialSeq;
    for (const entry of (logs.entries || [])) {
      state.serialLogs.push(`[${formatTimestamp(entry.timestamp)}] ${entry.direction} ${entry.bytes}B: ${entry.text || ''} | 十六进制: ${entry.hex || ''}`);
    }
    if (state.serialLogs.length > 300) state.serialLogs = state.serialLogs.slice(-300);
    el('serial-log').textContent = state.serialLogs.length ? state.serialLogs.join('\n') : '暂无日志';
  } catch (e) {
    el('serial-status').innerHTML = `<span class="err">串口状态刷新失败: ${e.message}</span>`;
  }
}

function clearSerialLogs() {
  state.serialLogs = [];
  el('serial-log').textContent = '暂无日志';
}

function refreshCurrentTab() {
  if (state.activeTab === 'dashboard') refreshDashboardFallback();
  if (state.activeTab === 'devices') refreshDevices();
  if (state.activeTab === 'control') refreshControlDevices();
  if (state.activeTab === 'device-debug') refreshDebugDevices();
  if (state.activeTab === 'protocols') refreshProtocols();
  if (state.activeTab === 'serial-debug') refreshSerialRuntime();
}

function bindEvents() {
  el('create-device-btn').addEventListener('click', createDevice);
  el('new-device-protocol').addEventListener('change', () => applyDeviceTemplateDefaults('new'));
  el('edit-device-id').addEventListener('change', loadEditDevice);
  el('edit-device-protocol').addEventListener('change', () => applyDeviceTemplateDefaults('edit'));
  el('update-device-btn').addEventListener('click', updateDevice);
  el('test-device-connection-btn').addEventListener('click', testDeviceConnection);
  el('control-device').addEventListener('change', loadControlSteps);
  el('control-tare').addEventListener('click', () => runControl('tare'));
  el('control-zero').addEventListener('click', () => runControl('zero'));
  el('control-run').addEventListener('click', () => runControl('custom'));
  el('debug-device').addEventListener('change', refreshDebugRuntime);
  el('debug-run').addEventListener('click', runDebugAction);

  el('create-protocol-btn').addEventListener('click', createProtocol);
  el('new-protocol-template').addEventListener('input', () => onProtocolTemplateInput('new'));
  el('edit-protocol-template').addEventListener('input', () => onProtocolTemplateInput('edit'));
  el('new-protocol-template').addEventListener('scroll', () => renderProtocolEditorVisual('new', state.protocolEditors.new.highlightLine));
  el('edit-protocol-template').addEventListener('scroll', () => renderProtocolEditorVisual('edit', state.protocolEditors.edit.highlightLine));
  el('new-protocol-template-gutter').addEventListener('click', e => {
    const line = Number(e.target?.dataset?.line || 0);
    if (line > 0) focusEditorAt(el('new-protocol-template'), line, 1);
  });
  el('edit-protocol-template-gutter').addEventListener('click', e => {
    const line = Number(e.target?.dataset?.line || 0);
    if (line > 0) focusEditorAt(el('edit-protocol-template'), line, 1);
  });
  el('new-protocol-name').addEventListener('input', () => syncProtocolTemplateFromFields('new'));
  el('new-protocol-desc').addEventListener('input', () => syncProtocolTemplateFromFields('new'));
  el('new-protocol-type').addEventListener('change', () => syncProtocolTemplateFromFields('new'));
  el('edit-protocol-name').addEventListener('input', () => syncProtocolTemplateFromFields('edit'));
  el('edit-protocol-desc').addEventListener('input', () => syncProtocolTemplateFromFields('edit'));
  el('edit-protocol-type').addEventListener('input', () => syncProtocolTemplateFromFields('edit'));
  el('new-protocol-format-btn').addEventListener('click', () => formatProtocolTemplate('new'));
  el('edit-protocol-format-btn').addEventListener('click', () => formatProtocolTemplate('edit'));
  el('edit-protocol-id').addEventListener('change', loadEditProtocol);
  el('update-protocol-btn').addEventListener('click', updateProtocol);
  el('delete-protocol-btn').addEventListener('click', deleteProtocol);
  el('step-test-btn').addEventListener('click', runStepTest);

  el('serial-refresh').addEventListener('click', refreshSerialPorts);
  el('serial-open').addEventListener('click', serialOpen);
  el('serial-close').addEventListener('click', serialClose);
  el('serial-send').addEventListener('click', serialSend);
  el('serial-clear').addEventListener('click', clearSerialLogs);
}

async function boot() {
  setupTabs();
  bindEvents();
  initProtocolEditors();
  await loadProtocolOptions();
  await refreshDashboardFallback();
  openDashboardWS();
  await refreshSerialPorts();
  setInterval(() => {
    if (state.activeTab === 'dashboard') refreshDashboardFallback();
  }, 10000);
  setInterval(() => {
    if (state.activeTab === 'dashboard') refreshDashboardMetrics();
    if (state.activeTab === 'devices') refreshDevices();
    if (state.activeTab === 'control') refreshControlDevices();
    if (state.activeTab === 'device-debug') refreshDebugRuntime();
    if (state.activeTab === 'serial-debug') refreshSerialRuntime();
  }, 1200);
}

boot();
