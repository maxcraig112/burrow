// Helpers + the Alpine component for the receive-web upload page.

function fmtBytes(n) {
  if (n < 1024) return n + ' B';
  const u = ['KB', 'MB', 'GB', 'TB'];
  let i = -1;
  do { n /= 1024; i++; } while (n >= 1024 && i < u.length - 1);
  return n.toFixed(1) + ' ' + u[i];
}

document.addEventListener('alpine:init', () => {
  Alpine.data('uploadForm', () => ({
    files: [],
    status: '',
    statusClass: '',
    fmtBytes,

    add(list) {
      for (const f of list) {
        if (!this.files.some(c => c.name === f.name && c.size === f.size)) this.files.push(f);
      }
      this.status = '';
    },
    remove(i) { this.files.splice(i, 1); },

    async upload() {
      const queue = [...this.files];
      this.files = [];
      for (const file of queue) {
        this.status = 'Uploading ' + file.name + '…';
        this.statusClass = '';
        try {
          const fd = new FormData();
          fd.append('file', file, file.name);
          const res = await fetch('upload', { method: 'POST', body: fd });
          if (!res.ok) throw new Error('HTTP ' + res.status);
          this.status = '✓ ' + file.name + ' delivered';
          this.statusClass = 'ok';
        } catch (err) {
          this.status = '✗ ' + err.message;
          this.statusClass = 'err';
          return;
        }
      }
    },
  }));
});
