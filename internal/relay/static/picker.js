let _pickerCallback = null;

async function _browseTo(path) {
  const pickerList = document.getElementById('picker-list');
  const pickerPath = document.getElementById('picker-path');
  pickerList.innerHTML = '<div class="picker-empty">Loading…</div>';
  pickerPath.textContent = path || '…';
  try {
    const res = await fetch('/api/browse' + (path ? '?path=' + encodeURIComponent(path) : ''));
    const data = await res.json();
    if (data.error) {
      pickerList.innerHTML = '<div class="picker-empty">' + esc(data.error) + '</div>';
      return;
    }
    pickerPath.textContent = data.path;
    document.getElementById('picker-select').dataset.path = data.path;
    pickerList.innerHTML = '';

    if (data.parent) {
      const up = document.createElement('div');
      up.className = 'picker-item';
      up.innerHTML = '<span class="icon">↑</span> ..';
      up.addEventListener('click', () => _browseTo(data.parent));
      pickerList.appendChild(up);
    }

    if (!data.dirs || data.dirs.length === 0) {
      const em = document.createElement('div');
      em.className = 'picker-empty';
      em.textContent = 'No subdirectories';
      pickerList.appendChild(em);
    } else {
      data.dirs.forEach(name => {
        const item = document.createElement('div');
        item.className = 'picker-item';
        item.innerHTML = '<span class="icon">📁</span>' + esc(name);
        const fullPath = data.path.replace(/[/\\]$/, '') + '/' + name;
        item.addEventListener('click', () => _browseTo(fullPath));
        pickerList.appendChild(item);
      });
    }
  } catch (err) {
    pickerList.innerHTML = '<div class="picker-empty">Failed: ' + esc(err.message) + '</div>';
  }
}

function openPicker(startPath, onSelect) {
  _pickerCallback = onSelect;
  document.getElementById('overlay').classList.add('open');
  _browseTo(startPath || '');
}

function closePicker() {
  document.getElementById('overlay').classList.remove('open');
}

document.addEventListener('DOMContentLoaded', () => {
  document.getElementById('picker-select').addEventListener('click', () => {
    const path = document.getElementById('picker-select').dataset.path;
    if (!path) return;
    closePicker();
    if (_pickerCallback) _pickerCallback(path);
  });
  document.getElementById('picker-close').addEventListener('click', closePicker);
  document.getElementById('picker-cancel').addEventListener('click', closePicker);
  document.getElementById('overlay').addEventListener('click', e => {
    if (e.target === document.getElementById('overlay')) closePicker();
  });
});
