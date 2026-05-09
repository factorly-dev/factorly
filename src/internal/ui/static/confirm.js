// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

// Confirm modal — SSE stream for shadow confirm prompts
function _initConfirmSSE() {
  // Reconnect if previous connection was closed (e.g., by auth redirect)
  if (window._confirmSSE && window._confirmSSE.readyState === EventSource.CLOSED) {
    window._confirmSSE = null;
  }
  if (window._confirmSSE) return;
  window._confirmSSE = new EventSource('/confirm/stream');
  window._confirmSSE.onmessage = function(event) {
    const pending = JSON.parse(event.data);
    const modal = document.getElementById('confirm-modal');
    if (!pending || pending.length === 0) {
      modal.classList.add('hidden');
      return;
    }
    const req = pending[0];
    let paramsHtml = '';
    if (req.params && Object.keys(req.params).length > 0) {
      paramsHtml = '<div class="mt-3 space-y-1">';
      for (const [k, v] of Object.entries(req.params)) {
        paramsHtml += `<div class="flex gap-2 text-xs"><span class="text-gray-500 font-medium w-24 shrink-0">${escapeHtml(k)}</span><span class="font-mono text-gray-700 truncate">${escapeHtml(v)}</span></div>`;
      }
      paramsHtml += '</div>';
    }
    document.getElementById('confirm-content').innerHTML = `
      <div class="flex items-center gap-2 mb-2">
        <span class="inline-flex items-center justify-center w-6 h-6 rounded-full bg-amber-100 text-amber-600 text-xs font-bold">!</span>
        <span class="font-mono text-sm font-medium">${escapeHtml(req.tool)}</span>
      </div>
      <p class="text-sm text-gray-600 mb-3">This tool requires confirmation before executing.</p>
      ${paramsHtml}
      <div class="flex gap-2 mt-4">
        <button onclick="respondConfirm('${req.id}', 'approve')" class="px-4 py-2 bg-green-600 text-white text-sm rounded hover:bg-green-700">Approve</button>
        <button onclick="respondConfirm('${req.id}', 'deny')" class="px-4 py-2 bg-red-50 text-red-700 text-sm rounded border border-red-200 hover:bg-red-100">Deny</button>
      </div>
    `;
    modal.classList.remove('hidden');
  };
}

function respondConfirm(id, action) {
  fetch(`/confirm/${id}/${action}`, { method: 'POST' });
  document.getElementById('confirm-modal').classList.add('hidden');
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', _initConfirmSSE);
} else {
  _initConfirmSSE();
}
