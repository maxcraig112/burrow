const drop      = document.getElementById('drop');
const browseBtn = document.getElementById('browse-btn');
const uploadBtn = document.getElementById('upload-btn');
const fileInput = document.getElementById('file-input');
const fileList  = document.getElementById('file-list');
const status    = document.getElementById('status');
const nameplate = window.NAMEPLATE;
let chosen = [];

function addFiles(files) {
  for (const f of Array.from(files)) {
    if (!chosen.some(c => c.name === f.name && c.size === f.size)) chosen.push(f);
  }
  fileInput.value = '';
  renderList();
  status.textContent = ''; status.className = 'status';
}

function removeFile(i) {
  chosen.splice(i, 1);
  renderList();
}

function renderList() {
  fileList.innerHTML = '';
  chosen.forEach((f, i) => {
    const item = document.createElement('div');
    item.className = 'file-item';
    const info = document.createElement('span');
    info.className = 'file-info';
    info.title = f.name;
    info.innerHTML = esc(f.name) + ' <span class="file-size">(' + fmtBytes(f.size) + ')</span>';
    const rm = document.createElement('button');
    rm.className = 'remove-btn'; rm.type = 'button'; rm.title = 'Remove'; rm.textContent = '✕';
    rm.addEventListener('click', () => removeFile(i));
    item.appendChild(info); item.appendChild(rm);
    fileList.appendChild(item);
  });
  uploadBtn.disabled = chosen.length === 0;
}

browseBtn.addEventListener('click', () => fileInput.click());
fileInput.addEventListener('change', () => addFiles(fileInput.files));
drop.addEventListener('dragover', e => { e.preventDefault(); drop.classList.add('over'); });
drop.addEventListener('dragleave', () => drop.classList.remove('over'));
drop.addEventListener('drop', e => { e.preventDefault(); drop.classList.remove('over'); addFiles(e.dataTransfer.files); });
drop.addEventListener('click', e => {
  if (e.target === drop || e.target.classList.contains('drop-icon') || e.target.classList.contains('drop-text'))
    fileInput.click();
});

document.getElementById('close-btn').addEventListener('click', async () => {
  if (!confirm('Close this session? Uploaders will no longer be able to send files.')) return;
  try {
    const res = await fetch('/api/close-session/' + nameplate, { method: 'POST' });
    if (!res.ok) { const d = await res.json().catch(() => ({})); alert(d.error || 'Failed to close session'); return; }
    window.location.href = '/';
  } catch (err) {
    alert('Request failed: ' + err.message);
  }
});

uploadBtn.addEventListener('click', async () => {
  if (!chosen.length) return;
  const files = [...chosen];
  chosen = []; renderList();

  for (const file of files) {
    status.className = 'status';
    status.textContent = 'Uploading ' + file.name + '…';
    try {
      const fd = new FormData();
      fd.append('file', file);
      const res = await fetch('upload', { method: 'POST', body: fd });
      if (!res.ok) throw new Error('HTTP ' + res.status);
      status.textContent = '✓ ' + file.name + ' delivered';
      status.className = 'status ok';
    } catch (err) {
      status.textContent = '✗ ' + err.message;
      status.className = 'status err';
      return;
    }
  }
});
