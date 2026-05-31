function metaRow(label, value) {
  return '<div><div class="ml">' + label + '</div><div class="mv">' + esc(value) + '</div></div>';
}

function setStatus(el, state, text) {
  el.className = 'status' + (state === 'ok' ? ' ok' : state === 'err' ? ' err' : '');
  if (state === 'uploading') {
    el.innerHTML = '<div class="spinner"></div>' + esc(text);
  } else {
    el.textContent = (state === 'ok' ? '✓ ' : state === 'err' ? '✗ ' : '') + text;
  }
}

function buildCard(s) {
  const card = document.createElement('div');
  card.className = 'card';
  card.dataset.np = s.nameplate;

  let head = '<div class="card-head"><span class="np">' + esc(s.nameplate) + '</span>';
  if (s.description) head += '<span class="desc">"' + esc(s.description) + '"</span>';
  head += '</div>';

  let meta = '<div class="meta">';
  meta += metaRow('Active for', dur(s.started_at));
  meta += metaRow('Files received', s.files_received);
  meta += metaRow('Receiver IP', s.receiver_ip);
  if (s.last_uploader_ip) meta += metaRow('Last upload from', s.last_uploader_ip);
  meta += '</div>';

  const dropZone =
    '<div class="drop-zone">' +
      '<div class="drop-icon">📂</div>' +
      '<div class="drop-text">Drop files here to upload</div>' +
    '</div>';

  card.innerHTML = head + meta + dropZone + '<div class="status"></div>';
  card.addEventListener('click', () => window.location.href = s.url);

  let dragN = 0;
  card.addEventListener('dragenter', e => { e.preventDefault(); dragN++; card.classList.add('drag-over'); });
  card.addEventListener('dragover', e => e.preventDefault());
  card.addEventListener('dragleave', () => { if (--dragN <= 0) { dragN = 0; card.classList.remove('drag-over'); } });
  card.addEventListener('drop', e => {
    e.preventDefault(); e.stopPropagation();
    dragN = 0; card.classList.remove('drag-over');
    const files = [...e.dataTransfer.files];
    if (files.length) uploadToSession(s.nameplate, files, card.querySelector('.status'));
  });

  return card;
}

async function uploadToSession(nameplate, files, statusEl) {
  const label = files.length + ' file' + (files.length > 1 ? 's' : '');
  setStatus(statusEl, 'uploading', 'Uploading ' + label + '…');
  const fd = new FormData();
  files.forEach(f => fd.append('file', f, f.name));
  try {
    const res = await fetch('/t/' + nameplate + '/upload', { method: 'POST', body: fd });
    if (!res.ok) throw new Error('HTTP ' + res.status);
    const data = await res.json().catch(() => ({}));
    const saved = data.saved ? data.saved.length : files.length;
    setStatus(statusEl, 'ok', saved + ' file' + (saved > 1 ? 's' : '') + ' uploaded');
    setTimeout(() => { if (statusEl.className.includes('ok')) setStatus(statusEl, '', ''); }, 5000);
    loadSessions();
  } catch (err) {
    setStatus(statusEl, 'err', 'Failed: ' + err.message);
  }
}

async function loadSessions() {
  try {
    const res = await fetch('/api/sessions');
    const data = await res.json();
    const sessions = (data.sessions || []).sort((a, b) => new Date(a.started_at) - new Date(b.started_at));
    const root = document.getElementById('root');

    const prev = {};
    root.querySelectorAll('.card').forEach(c => {
      const el = c.querySelector('.status');
      if (el.textContent || el.innerHTML) prev[c.dataset.np] = { html: el.innerHTML, cls: el.className };
    });

    root.innerHTML = '';
    if (!sessions.length) {
      root.innerHTML = '<div class="empty">No active sessions</div>';
      return;
    }
    sessions.forEach(s => {
      const card = buildCard(s);
      if (prev[s.nameplate]) {
        const el = card.querySelector('.status');
        el.innerHTML = prev[s.nameplate].html;
        el.className = prev[s.nameplate].cls;
      }
      root.appendChild(card);
    });
  } catch (e) { console.error('sessions fetch failed', e); }
}

loadSessions();
setInterval(loadSessions, 10000);
