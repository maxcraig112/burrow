let _chosenDir = '';

function _setDir(path) {
  _chosenDir = path;
  const display = document.getElementById('dir-display');
  display.textContent = path;
  display.classList.remove('placeholder');
  document.getElementById('create-btn').disabled = false;
}

async function _prefill() {
  try {
    const res = await fetch('/api/config');
    const cfg = await res.json();
    if (cfg.upload_dir && !_chosenDir) _setDir(cfg.upload_dir);
  } catch (_) {}
}

document.addEventListener('DOMContentLoaded', () => {
  _prefill();

  document.getElementById('browse-btn').addEventListener('click', () => {
    openPicker(_chosenDir, _setDir);
  });

  document.getElementById('create-btn').addEventListener('click', async () => {
    const desc = document.getElementById('desc-input').value.trim();
    const errEl = document.getElementById('err-msg');
    errEl.textContent = '';
    if (!_chosenDir) { errEl.textContent = 'Please choose a directory.'; return; }

    const btn = document.getElementById('create-btn');
    btn.disabled = true;
    try {
      const res = await fetch('/api/create-session', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ directory: _chosenDir, description: desc || undefined })
      });
      const data = await res.json();
      if (!res.ok) { errEl.textContent = data.error || 'Server error'; btn.disabled = false; return; }

      document.getElementById('result-np').textContent = data.nameplate;
      document.getElementById('result-url').innerHTML =
        '<a href="' + data.url + '" target="_blank">' + data.url + '</a>';

      const qrBox = document.getElementById('qr-box');
      qrBox.innerHTML = '';
      const img = document.createElement('img');
      img.src = '/api/qr?url=' + encodeURIComponent(data.url);
      img.width = 200;
      img.height = 200;
      img.style.borderRadius = '8px';
      qrBox.appendChild(img);

      document.getElementById('result').style.display = 'block';
      btn.textContent = 'Create Another';
      btn.disabled = false;
      document.getElementById('desc-input').value = '';
    } catch (err) {
      errEl.textContent = 'Request failed: ' + err.message;
      btn.disabled = false;
    }
  });
});
