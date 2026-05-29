// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

// Global toast notifications. A toast is delivered TWO ways by the
// server (see toast() in toast.go): an HX-Trigger header (fires now,
// for fragment swaps) and a flash cookie (read on the next page, for
// redirects). Both carry the same id; we dedupe on it so each toast
// pops exactly once no matter which delivery — or both — arrive.
(function() {
  if (window._toastInit) return;
  window._toastInit = true;

  var STYLES = {
    success: 'bg-green-600 text-white',
    error: 'bg-red-600 text-white',
    info: 'bg-gray-800 text-white',
  };

  // FIFO set of recently-seen toast ids. Bounded so it can't grow
  // unbounded over a long session; 20 is far more than the handful
  // of deliveries that can be in flight at once.
  var seen = [];
  var SEEN_CAP = 20;

  function markSeen(id) {
    if (!id) return false; // no id → can't dedupe; treat as unseen
    if (seen.indexOf(id) !== -1) return true;
    seen.push(id);
    if (seen.length > SEEN_CAP) seen.shift();
    return false;
  }

  // popToast renders the chip unless this id was already shown.
  // window.toast(msg, kind) is the public API (id optional — callers
  // like confirm.js pass none and always pop).
  window.toast = function(msg, kind, id) {
    if (!msg) return;
    if (markSeen(id)) return; // already popped this exact toast
    var cls = STYLES[kind] || STYLES.info;
    var t = document.createElement('div');
    t.className = 'fixed top-4 right-4 z-50 px-3 py-2 text-xs rounded shadow-lg ' + cls;
    t.textContent = msg;
    document.body.appendChild(t);
    setTimeout(function() { t.remove(); }, 2500);
  };

  // Delivery 1 — HX-Trigger header. HTMX dispatches a bubbling "toast"
  // event with detail {id, msg, kind}.
  document.body.addEventListener('toast', function(e) {
    var d = (e && e.detail) || {};
    window.toast(d.msg, d.kind, d.id);
  });

  // Delivery 2 — flash cookie "id|kind|msg" (URL-encoded), read on
  // page load and after every htmx swap. Clear it once read so it
  // can't re-fire on a later navigation; dedupe-by-id covers the
  // race where the HX-Trigger already popped this same toast.
  function readFlash() {
    var m = document.cookie.match(/(?:^|;\s*)factorly_flash=([^;]*)/);
    if (!m) return;
    document.cookie = 'factorly_flash=; Path=/; Max-Age=0; SameSite=Lax';
    var raw;
    try { raw = decodeURIComponent(m[1]); } catch (_) { return; }
    // Split into at most 3 parts: id, kind, msg (msg may contain '|').
    var first = raw.indexOf('|');
    if (first < 0) return;
    var second = raw.indexOf('|', first + 1);
    if (second < 0) return;
    var id = raw.slice(0, first);
    var kind = raw.slice(first + 1, second);
    var msg = raw.slice(second + 1);
    window.toast(msg, kind, id);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', readFlash);
  } else {
    readFlash();
  }
  document.body.addEventListener('htmx:afterSettle', readFlash);
})();
