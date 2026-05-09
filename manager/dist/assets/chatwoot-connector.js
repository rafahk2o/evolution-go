(function () {
  'use strict';

  const STYLE_ID = 'cw-connector-style';
  const MODAL_ID = 'cw-connector-modal';
  const CARD_FLAG = 'cwBound';

  let instanceCache = null;
  let cacheTime = 0;
  let inFlight = null;

  function injectStyles() {
    if (document.getElementById(STYLE_ID)) return;
    const style = document.createElement('style');
    style.id = STYLE_ID;
    style.textContent = `
      .cw-card-btn {
        position: absolute;
        bottom: 56px;
        right: 10px;
        z-index: 5;
        background: rgba(45, 154, 211, 0.18);
        color: #2d9ad3;
        border: 1px solid rgba(45, 154, 211, 0.5);
        border-radius: 6px;
        padding: 4px 9px;
        font-size: 11px;
        font-weight: 600;
        cursor: pointer;
        display: inline-flex;
        align-items: center;
        gap: 5px;
        line-height: 1;
        backdrop-filter: blur(4px);
        transition: background 0.15s ease, transform 0.15s ease;
      }
      .cw-card-btn:hover { background: rgba(45, 154, 211, 0.35); transform: translateY(-1px); }
      .cw-card-btn svg { display: block; }

      .cw-overlay {
        position: fixed; inset: 0;
        background: rgba(0, 0, 0, 0.65);
        display: flex; align-items: center; justify-content: center;
        z-index: 9999;
        font-family: inherit;
      }
      .cw-modal {
        background: #1a1d23;
        color: #e6e8eb;
        border: 1px solid rgba(255,255,255,0.08);
        border-radius: 12px;
        width: min(92vw, 500px);
        max-height: 92vh;
        overflow-y: auto;
        padding: 22px 22px 18px;
        box-shadow: 0 20px 60px rgba(0,0,0,0.5);
      }
      .cw-modal-header {
        display: flex; justify-content: space-between; align-items: center;
        margin-bottom: 18px; padding-bottom: 12px;
        border-bottom: 1px solid rgba(255,255,255,0.08);
      }
      .cw-modal-header h2 { margin: 0; font-size: 17px; font-weight: 600; }
      .cw-modal-header h2 small { font-weight: 400; opacity: 0.6; font-size: 13px; }
      .cw-close {
        background: none; border: none; color: inherit;
        font-size: 24px; line-height: 1; cursor: pointer; padding: 0 4px;
      }
      .cw-close:hover { opacity: 0.7; }

      .cw-section-title {
        font-size: 11px; font-weight: 700; text-transform: uppercase;
        letter-spacing: 0.06em; opacity: 0.65;
        margin: 16px 0 8px;
      }
      .cw-field { margin-bottom: 10px; }
      .cw-field label { display: block; font-size: 12px; margin-bottom: 5px; opacity: 0.85; }
      .cw-field input[type="text"], .cw-field input[type="password"] {
        width: 100%; padding: 9px 10px; box-sizing: border-box;
        background: rgba(255,255,255,0.04);
        border: 1px solid rgba(255,255,255,0.12);
        color: inherit; border-radius: 6px; font-size: 13px;
        font-family: inherit;
      }
      .cw-field input:focus { outline: none; border-color: #2d9ad3; }

      .cw-row { display: flex; gap: 8px; }
      .cw-row .cw-field { flex: 1; }

      .cw-toggle {
        display: inline-flex; align-items: center; gap: 8px;
        cursor: pointer; user-select: none;
        background: rgba(255,255,255,0.04);
        padding: 7px 12px; border-radius: 6px;
        border: 1px solid rgba(255,255,255,0.12);
        font-size: 13px;
      }
      .cw-toggle input { accent-color: #2d9ad3; margin: 0; }

      .cw-webhook {
        display: flex; gap: 6px; align-items: center;
        background: rgba(255,255,255,0.04);
        border: 1px solid rgba(255,255,255,0.12);
        border-radius: 6px; padding: 4px 4px 4px 8px;
      }
      .cw-webhook input {
        flex: 1; background: transparent; border: none; color: inherit;
        font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
        font-size: 11px; padding: 6px 0;
      }
      .cw-webhook input:focus { outline: none; }
      .cw-webhook .cw-btn-copy {
        background: rgba(45,154,211,0.2); color: #2d9ad3;
        padding: 5px 12px; font-size: 11px; font-weight: 600;
      }

      .cw-btn {
        padding: 9px 16px; border-radius: 6px; cursor: pointer;
        font-size: 13px; font-weight: 500; border: none;
        font-family: inherit;
        transition: background 0.15s ease;
      }
      .cw-btn:disabled { opacity: 0.6; cursor: progress; }
      .cw-btn-primary { background: #2d9ad3; color: white; }
      .cw-btn-primary:hover:not(:disabled) { background: #2589bf; }
      .cw-btn-secondary {
        background: transparent; color: inherit;
        border: 1px solid rgba(255,255,255,0.18);
      }
      .cw-btn-secondary:hover { background: rgba(255,255,255,0.05); }

      .cw-actions {
        display: flex; gap: 8px; justify-content: flex-end;
        margin-top: 18px; padding-top: 14px;
        border-top: 1px solid rgba(255,255,255,0.08);
      }

      .cw-msg { font-size: 12px; margin-top: 10px; min-height: 16px; }
      .cw-msg.error { color: #ef4444; }
      .cw-msg.success { color: #10b981; }

      .cw-loading { padding: 24px; text-align: center; opacity: 0.7; font-size: 13px; }
    `;
    document.head.appendChild(style);
  }

  function getAuth() {
    try {
      const raw = localStorage.getItem('evolution-auth');
      if (!raw) return null;
      const parsed = JSON.parse(raw);
      const state = parsed.state || parsed;
      if (!state || !state.apiKey) return null;
      return { apiUrl: (state.apiUrl || '').replace(/\/+$/, ''), apiKey: state.apiKey };
    } catch {
      return null;
    }
  }

  async function fetchInstances() {
    const auth = getAuth();
    if (!auth) return null;
    if (instanceCache && Date.now() - cacheTime < 30000) return instanceCache;
    if (inFlight) return inFlight;
    inFlight = (async () => {
      try {
        const res = await fetch(auth.apiUrl + '/instance/all', {
          headers: { apikey: auth.apiKey, 'Content-Type': 'application/json' },
        });
        if (!res.ok) return null;
        const json = await res.json();
        const list = (json && json.data) || [];
        const map = {};
        list.forEach(i => {
          if (i && i.name) map[i.name] = i;
        });
        instanceCache = map;
        cacheTime = Date.now();
        return map;
      } catch {
        return null;
      } finally {
        inFlight = null;
      }
    })();
    return inFlight;
  }

  function classOf(el) {
    if (!el) return '';
    return typeof el.className === 'string' ? el.className : (el.getAttribute && el.getAttribute('class')) || '';
  }

  function isInstanceCard(el) {
    if (!(el instanceof Element)) return false;
    const cls = classOf(el);
    return (
      cls.indexOf('group') !== -1 &&
      cls.indexOf('bg-sidebar') !== -1 &&
      cls.indexOf('border-sidebar-border') !== -1 &&
      cls.indexOf('overflow-hidden') !== -1
    );
  }

  function findCardName(card) {
    const ps = card.querySelectorAll('p.truncate');
    for (let i = 0; i < ps.length; i++) {
      const cls = classOf(ps[i]);
      if (cls.indexOf('text-xs') !== -1 && cls.indexOf('text-sidebar-foreground/60') !== -1) {
        return (ps[i].textContent || '').trim();
      }
    }
    const h3 = card.querySelector('h3');
    return h3 ? (h3.textContent || '').trim() : '';
  }

  async function injectButtons() {
    const candidates = document.querySelectorAll('.group.bg-sidebar');
    if (!candidates.length) return;
    let map = null;
    for (let i = 0; i < candidates.length; i++) {
      const card = candidates[i];
      if (!isInstanceCard(card)) continue;
      if (card.dataset[CARD_FLAG] === '1') continue;
      const name = findCardName(card);
      if (!name) continue;

      if (!map) {
        map = await fetchInstances();
        if (!map) return;
      }
      const data = map[name];
      if (!data || !data.id) continue;

      card.dataset[CARD_FLAG] = '1';
      const btn = document.createElement('button');
      btn.className = 'cw-card-btn';
      btn.type = 'button';
      btn.title = 'Configurar Chatwoot';
      btn.innerHTML =
        '<svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>Chatwoot';
      btn.addEventListener('click', e => {
        e.preventDefault();
        e.stopPropagation();
        openModal(data);
      });
      card.appendChild(btn);
    }
  }

  function escapeHtml(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, c => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
    }[c]));
  }

  function openModal(instance) {
    closeModal();
    const auth = getAuth();
    if (!auth) return;

    const overlay = document.createElement('div');
    overlay.id = MODAL_ID;
    overlay.className = 'cw-overlay';
    overlay.addEventListener('click', e => {
      if (e.target === overlay) closeModal();
    });

    overlay.innerHTML =
      '<div class="cw-modal" role="dialog" aria-modal="true">' +
      '<div class="cw-modal-header">' +
      '<h2>Chatwoot <small>· ' + escapeHtml(instance.name) + '</small></h2>' +
      '<button class="cw-close" type="button" aria-label="Fechar">&times;</button>' +
      '</div>' +
      '<div class="cw-body"><div class="cw-loading">Carregando configurações...</div></div>' +
      '</div>';

    document.body.appendChild(overlay);
    overlay.querySelector('.cw-close').addEventListener('click', closeModal);
    document.addEventListener('keydown', escClose);

    loadAndRender(overlay, instance, auth);
  }

  function escClose(e) {
    if (e.key === 'Escape') closeModal();
  }

  function closeModal() {
    const m = document.getElementById(MODAL_ID);
    if (m) m.remove();
    document.removeEventListener('keydown', escClose);
  }

  async function loadAndRender(overlay, instance, auth) {
    const body = overlay.querySelector('.cw-body');
    try {
      const res = await fetch(auth.apiUrl + '/instance/' + encodeURIComponent(instance.id) + '/chatwoot-settings', {
        headers: { apikey: instance.token, 'Content-Type': 'application/json' },
      });
      if (!res.ok) {
        const txt = await res.text().catch(() => '');
        body.innerHTML = '<div class="cw-msg error">Erro ao carregar (' + res.status + '): ' + escapeHtml(txt) + '</div>';
        return;
      }
      const data = await res.json();
      renderForm(body, instance, auth, data.settings || {}, data.webhookUrl || '');
    } catch (e) {
      body.innerHTML = '<div class="cw-msg error">Erro: ' + escapeHtml(e && e.message ? e.message : String(e)) + '</div>';
    }
  }

  function renderForm(body, instance, auth, settings, webhookUrl) {
    body.innerHTML =
      '<label class="cw-toggle">' +
      '<input type="checkbox" id="cw-enabled" ' + (settings.enabled ? 'checked' : '') + ' />' +
      '<span>Habilitado</span>' +
      '</label>' +

      '<div class="cw-section-title">Webhook URL para o Inbox</div>' +
      '<div class="cw-webhook">' +
      '<input id="cw-webhook" readonly value="' + escapeHtml(webhookUrl) + '" />' +
      '<button class="cw-btn cw-btn-copy" type="button" id="cw-copy">Copiar</button>' +
      '</div>' +

      '<div class="cw-section-title">Conexão</div>' +
      '<div class="cw-field">' +
      '<label>URL do Chatwoot</label>' +
      '<input id="cw-url" type="text" placeholder="https://app.chatwoot.com" value="' + escapeHtml(settings.url) + '" />' +
      '</div>' +
      '<div class="cw-row">' +
      '<div class="cw-field">' +
      '<label>Account ID</label>' +
      '<input id="cw-accountId" type="text" placeholder="1" value="' + escapeHtml(settings.accountId) + '" />' +
      '</div>' +
      '<div class="cw-field">' +
      '<label>Inbox ID</label>' +
      '<input id="cw-inboxId" type="text" placeholder="ID do inbox" value="' + escapeHtml(settings.inboxId) + '" />' +
      '</div>' +
      '</div>' +
      '<div class="cw-field">' +
      '<label>Account Token (User API Key)</label>' +
      '<input id="cw-accountToken" type="password" placeholder="Chatwoot user access token" value="' + escapeHtml(settings.accountToken) + '" />' +
      '</div>' +
      '<div class="cw-field">' +
      '<label>Inbox Identifier</label>' +
      '<input id="cw-inboxIdentifier" type="password" placeholder="Identifier da API Inbox" value="' + escapeHtml(settings.inboxIdentifier) + '" />' +
      '</div>' +

      '<div class="cw-section-title">Segurança (opcional)</div>' +
      '<div class="cw-field">' +
      '<label>Webhook Token (sobrescreve token da instância na URL)</label>' +
      '<input id="cw-webhookToken" type="text" value="' + escapeHtml(settings.webhookToken) + '" />' +
      '</div>' +
      '<div class="cw-field">' +
      '<label>HMAC Token (assina mensagens recebidas do Chatwoot)</label>' +
      '<input id="cw-hmacToken" type="password" value="' + escapeHtml(settings.hmacToken) + '" />' +
      '</div>' +

      '<label class="cw-toggle" style="margin-top:8px;">' +
      '<input type="checkbox" id="cw-enableGroups" ' + (settings.enableGroups ? 'checked' : '') + ' />' +
      '<span>Sincronizar grupos do WhatsApp</span>' +
      '</label>' +

      '<div id="cw-msg" class="cw-msg"></div>' +

      '<div class="cw-actions">' +
      '<button class="cw-btn cw-btn-secondary" type="button" id="cw-cancel">Cancelar</button>' +
      '<button class="cw-btn cw-btn-primary" type="button" id="cw-save">Salvar</button>' +
      '</div>';

    body.querySelector('#cw-copy').addEventListener('click', async () => {
      const v = body.querySelector('#cw-webhook').value;
      try {
        await navigator.clipboard.writeText(v);
        flash(body, 'Webhook copiado.', 'success');
      } catch {
        flash(body, 'Não foi possível copiar.', 'error');
      }
    });
    body.querySelector('#cw-cancel').addEventListener('click', closeModal);
    body.querySelector('#cw-save').addEventListener('click', () => save(body, instance, auth));
  }

  async function save(body, instance, auth) {
    const enabled = body.querySelector('#cw-enabled').checked;
    const payload = {
      enabled,
      url: body.querySelector('#cw-url').value.trim(),
      accountId: body.querySelector('#cw-accountId').value.trim(),
      accountToken: body.querySelector('#cw-accountToken').value.trim(),
      inboxId: body.querySelector('#cw-inboxId').value.trim(),
      inboxIdentifier: body.querySelector('#cw-inboxIdentifier').value.trim(),
      webhookToken: body.querySelector('#cw-webhookToken').value.trim(),
      hmacToken: body.querySelector('#cw-hmacToken').value.trim(),
      enableGroups: body.querySelector('#cw-enableGroups').checked,
    };

    if (enabled && !payload.url) {
      flash(body, 'URL do Chatwoot é obrigatória quando habilitado.', 'error');
      return;
    }
    if (enabled && !payload.inboxIdentifier) {
      flash(body, 'Inbox Identifier é obrigatório quando habilitado.', 'error');
      return;
    }

    const saveBtn = body.querySelector('#cw-save');
    saveBtn.disabled = true;
    saveBtn.textContent = 'Salvando...';
    try {
      const res = await fetch(auth.apiUrl + '/instance/' + encodeURIComponent(instance.id) + '/chatwoot-settings', {
        method: 'PUT',
        headers: { apikey: instance.token, 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        flash(body, data.error || ('Erro ao salvar (' + res.status + ').'), 'error');
        return;
      }
      if (data.webhookUrl) {
        const w = body.querySelector('#cw-webhook');
        if (w) w.value = data.webhookUrl;
      }
      flash(body, 'Configurações salvas.', 'success');
    } catch (e) {
      flash(body, 'Erro: ' + (e && e.message ? e.message : String(e)), 'error');
    } finally {
      saveBtn.disabled = false;
      saveBtn.textContent = 'Salvar';
    }
  }

  function flash(body, msg, type) {
    const el = body.querySelector('#cw-msg');
    if (!el) return;
    el.className = 'cw-msg ' + type;
    el.textContent = msg;
    const stamp = msg;
    setTimeout(() => {
      if (el.textContent === stamp) {
        el.textContent = '';
        el.className = 'cw-msg';
      }
    }, 3500);
  }

  function start() {
    injectStyles();
    let scheduled = false;
    const schedule = () => {
      if (scheduled) return;
      scheduled = true;
      requestAnimationFrame(() => {
        scheduled = false;
        injectButtons();
      });
    };
    const observer = new MutationObserver(schedule);
    observer.observe(document.body, { childList: true, subtree: true });
    schedule();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', start);
  } else {
    start();
  }
})();
