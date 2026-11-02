'use strict';

const element = id => document.getElementById(id);
const storageKey = 'oj-m0-fixed-operation-v1';
const version = 'm0-sum-v3';
const keyPattern = /^[A-Za-z0-9_-]{16,128}$/;
const stateLabels = { queued: '排队中', running: '判题中', finished: '已完成', failed: '任务未完成' };
const life = new AbortController();
let nonce = '';
let presets = [];
let operation = null;
let receipt = null;
let busy = false;
let halted = false;
let retryable = false;
let storageBlocked = false;

window.addEventListener('pagehide', () => { halted = true; life.abort(); });
window.addEventListener('pageshow', event => { if (event.persisted) location.reload(); });

function terminal() { return receipt && ['finished', 'failed'].includes(receipt.state); }
function message(text) { element('status').textContent = text; }

function render() {
  element('preset').disabled = !nonce || busy || !!operation || storageBlocked;
  element('submit').disabled = !nonce || busy || !!operation || storageBlocked;
  element('check').disabled = !nonce || busy || !operation || storageBlocked;
  element('retry').hidden = !operation || operation.binding === 'conflict' || (!retryable && operation.binding === 'confirmed');
  element('retry').disabled = busy || storageBlocked;
  element('stop').hidden = !busy;
  element('next').hidden = !terminal();
  element('next').disabled = busy || storageBlocked;
  const selected = presets.find(p => p.id === element('preset').value);
  if (selected) element('source').textContent = selected.source;
  element('receipt').hidden = !operation;
  element('operation-key').textContent = operation ? operation.key : '';
  element('submission-id').textContent = receipt ? receipt.id : '尚未核验';
  element('material-version').textContent = version;
  element('verdict').textContent = receipt && receipt.verdict ? receipt.verdict : '尚无判定';
}

function persist(value) {
  try {
    if (value) sessionStorage.setItem(storageKey, JSON.stringify(value));
    else sessionStorage.removeItem(storageKey);
    element('storage').textContent = '原键保存在当前标签页，刷新后会先核验原操作。';
    return true;
  } catch {
    element('storage').textContent = '标签页存储不可用，未发送新的提交。请恢复存储后原样重试；当前原键仍保留在本页。';
    return false;
  }
}

function restore() {
  try {
    const raw = sessionStorage.getItem(storageKey);
    if (!raw) return;
    const value = JSON.parse(raw);
    if (!value || Object.keys(value).sort().join(',') !== 'binding,key,preset,version' ||
      value.version !== version || !['sum', 'wrong'].includes(value.preset) ||
      !['unverified', 'confirmed', 'conflict'].includes(value.binding) ||
      typeof value.key !== 'string' || !keyPattern.test(value.key)) throw new Error('unverifiable operation');
    operation = value;
    element('preset').value = value.preset;
  } catch {
    storageBlocked = true;
    message('标签页中的原操作无法核验，已停止新提交。请保留原记录并检查存储；此页不会自动换键。');
  }
}

async function request(path, method = 'GET', body = null) {
  const headers = {};
  if (nonce) headers['X-OJ-M0-Nonce'] = nonce;
  if (operation) headers['Idempotency-Key'] = operation.key;
  if (body) headers['Content-Type'] = 'application/json';
  const response = await fetch(path, { method, headers, body: body ? JSON.stringify(body) : null,
    credentials: 'omit', cache: 'no-store', redirect: 'error',
    signal: AbortSignal.any([life.signal, AbortSignal.timeout(4000)]) });
  if (!response.body) throw new Error('missing response');
  const reader = response.body.getReader();
  const decoder = new TextDecoder('utf-8', { fatal: true });
  let text = '';
  let bytes = 0;
  try {
    for (;;) {
      const part = await reader.read();
      if (part.done) break;
      bytes += part.value.byteLength;
      if (bytes > 8192) throw new Error('response exceeded bound');
      text += decoder.decode(part.value, { stream: true });
    }
    text += decoder.decode();
  } finally { await reader.cancel(); reader.releaseLock(); }
  return { status: response.status, data: JSON.parse(text) };
}

function acceptReceipt(data, confirmed = false) {
  if (!data || Object.keys(data).sort().join(',') !== 'created_at,id,language,material_version,state,verdict' ||
    typeof data.id !== 'string' || !/^[a-f0-9]{32}$/.test(data.id) || data.material_version !== version ||
    data.language !== 'gnu++17' || typeof data.created_at !== 'string' || !Number.isFinite(Date.parse(data.created_at)) ||
    !['queued', 'running', 'finished', 'failed'].includes(data.state) ||
    (data.state === 'finished' ? !['AC', 'WA', 'CE', 'RE', 'TLE', 'MLE', 'OLE'].includes(data.verdict) : data.verdict !== null)) {
    throw new Error('invalid public receipt');
  }
  if (receipt && (receipt.id !== data.id || receipt.created_at !== data.created_at)) throw new Error('receipt changed');
  if (terminal() && (receipt.state !== data.state || receipt.verdict !== data.verdict)) throw new Error('terminal receipt changed');
  receipt = data;
  if (confirmed) { operation.binding = 'confirmed'; persist(operation); }
  retryable = false;
  if (operation.binding === 'conflict') message(`该原键存在内容冲突。查到的是既有记录（${stateLabels[data.state]}${data.verdict ? ' / ' + data.verdict : ''}），不能归为所选示例的结果。`);
  else if (operation.binding === 'unverified') message(`查到原键记录（${stateLabels[data.state]}${data.verdict ? ' / ' + data.verdict : ''}），尚未核验是否对应所选示例。可原样重试以核验内容，不会自动换键。`);
  else if (data.state === 'finished') message(`判题完成：${data.verdict}`);
  else if (data.state === 'failed') message('判题任务未完成，暂无选手判定。可核验原回执或开始下一次实验。');
  else if (data.state === 'queued') message('已接收，正在排队。尚无判定。');
  else message('正在判题。尚无判定。');
  render();
}

function rejected(result, submitting) {
  retryable = !receipt && result.status !== 409;
  if (result.status === 409) {
    operation.binding = 'conflict';
    persist(operation);
    message('原幂等键与已有内容冲突。已保留原操作，查询仅显示该键既有记录，不能归为所选示例；不会自动换键。');
  }
  else if (result.status === 404) message('暂未查到原键回执，这不表示提交已回滚。请继续核验，或使用原键和原示例重试。');
  else if (result.status === 403) message('本机实验入口校验失败。已保留原键，请刷新入口后继续核验。');
  else message(submitting ? '接收状态未知。已保留原键和原示例，请核验回执后再决定是否原样重试。' : '查询暂时失败，未改变判题结果。请使用原键继续核验。');
}

async function poll() {
  const started = Date.now();
  let count = 0;
  while (!halted && (count === 0 || !terminal()) && count++ < 40 && Date.now() - started < 30000) {
    const result = await request('/m0/api/receipt');
    if (result.status !== 200) { rejected(result, false); return; }
    acceptReceipt(result.data);
    if (!terminal() && !halted) await new Promise(resolve => setTimeout(resolve, 500));
  }
  if (!terminal()) message('自动查询已停止，判题任务不会因此取消。请使用原键继续核验。');
}

async function run(submitting) {
  if (busy || !nonce || !operation || storageBlocked) return;
  busy = true;
  halted = false;
  render();
  try {
    if (submitting) {
      operation.binding = 'unverified';
      if (!persist(operation)) { retryable = true; message('原操作尚未发送，标签页存储写入失败。请保留原键并恢复存储后原样重试。'); return; }
      message('正在提交固定示例…');
      const result = await request('/m0/api/submissions', 'POST', { preset: operation.preset });
      if (![200, 202].includes(result.status)) { rejected(result, true); return; }
      acceptReceipt(result.data, true);
    } else message('正在核验原键回执…');
    await poll();
  } catch {
    if (!life.signal.aborted) {
      retryable = !receipt;
      message(submitting && !receipt ? '接收状态未知。已保留原键和原示例，请核验回执；不会自动重新提交。' : '查询暂时失败，未改变判题结果。请使用原键继续核验。');
    }
  } finally { busy = false; render(); }
}

element('preset').addEventListener('change', render);
element('submit').addEventListener('click', () => {
  if (busy || operation || !nonce || storageBlocked) return;
  operation = { key: crypto.randomUUID(), preset: element('preset').value, version, binding: 'unverified' };
  render();
  void run(true);
});
element('retry').addEventListener('click', () => { void run(true); });
element('check').addEventListener('click', () => { void run(false); });
element('stop').addEventListener('click', () => { halted = true; message('正在停止查询；判题任务不会取消。'); });
element('next').addEventListener('click', () => {
  if (busy || !terminal() || !persist(null)) return;
  operation = null;
  receipt = null;
  retryable = false;
  message('尚未提交。请选择固定示例。');
  render();
});

async function start() {
  try {
    const result = await request('/m0/api/bootstrap');
    const data = result.data;
    if (result.status !== 200 || !data || data.version !== version || typeof data.nonce !== 'string' ||
      !/^[a-f0-9]{64}$/.test(data.nonce) || !Array.isArray(data.presets) || data.presets.length !== 2 ||
      data.presets.map(p => p.id).sort().join(',') !== 'sum,wrong' ||
      data.presets.some(p => typeof p.source !== 'string' || p.source.length > 2048)) throw new Error('invalid bootstrap');
    nonce = data.nonce;
    presets = data.presets;
    restore();
    render();
    if (storageBlocked) return;
    if (operation) await run(false);
    else { message('尚未提交。请选择固定示例。'); render(); }
  } catch {
    message('本机实验入口暂不可用。请刷新重试；已保存的原操作不会被替换。');
  }
}
void start();
