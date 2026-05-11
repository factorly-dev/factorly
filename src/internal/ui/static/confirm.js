// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

// Confirm modal — SSE stream for shadow confirm prompts with polling fallback
// Guard against hx-boost re-execution: state lives on window, not in the IIFE.
(function() {
  if (window._confirmInit) return;
  window._confirmInit = true;
  window._confirmSSE = null;
  window._confirmPollTimer = null;
  window._confirmReconnectTimer = null;
  var reconnectDelay = 1000;

  function renderConfirm(pending) {
    var modal = document.getElementById('confirm-modal');
    if (!modal) return;
    if (!pending || pending.length === 0) {
      modal.classList.add('hidden');
      return;
    }
    var req = pending[0];
    var paramsHtml = '';
    if (req.params && Object.keys(req.params).length > 0) {
      paramsHtml = '<div class="mt-3 space-y-1">';
      for (var k in req.params) {
        paramsHtml += '<div class="flex gap-2 text-xs"><span class="text-gray-500 font-medium w-24 shrink-0">' + escapeHtml(k) + '</span><span class="font-mono text-gray-700 truncate">' + escapeHtml(req.params[k]) + '</span></div>';
      }
      paramsHtml += '</div>';
    }
    document.getElementById('confirm-content').innerHTML =
      '<div class="flex items-center gap-2 mb-2">' +
        '<span class="inline-flex items-center justify-center w-6 h-6 rounded-full bg-amber-100 text-amber-600 text-xs font-bold">!</span>' +
        '<span class="font-mono text-sm font-medium">' + escapeHtml(req.tool) + '</span>' +
      '</div>' +
      '<p class="text-sm text-gray-600 mb-3">This tool requires confirmation before executing.</p>' +
      paramsHtml +
      '<div class="flex gap-2 mt-4">' +
        '<button onclick="respondConfirm(\'' + req.id + '\', \'approve\')" class="px-4 py-2 bg-green-600 text-white text-sm rounded hover:bg-green-700">Approve</button>' +
        '<button onclick="respondConfirm(\'' + req.id + '\', \'deny\')" class="px-4 py-2 bg-red-50 text-red-700 text-sm rounded border border-red-200 hover:bg-red-100">Deny</button>' +
      '</div>';
    modal.classList.remove('hidden');
  }

  function pollPending() {
    fetch('/confirm/pending')
      .then(function(r) { return r.ok ? r.json() : []; })
      .then(renderConfirm)
      .catch(function() {});
  }

  function startPolling() {
    if (window._confirmPollTimer) return;
    // Fallback only — runs when SSE is unavailable
    window._confirmPollTimer = setInterval(pollPending, 2000);
  }

  function stopPolling() {
    if (window._confirmPollTimer) {
      clearInterval(window._confirmPollTimer);
      window._confirmPollTimer = null;
    }
  }

  function connectSSE() {
    if (window._confirmSSE && window._confirmSSE.readyState !== EventSource.CLOSED) return;

    var sse = new EventSource('/confirm/stream');
    window._confirmSSE = sse;
    reconnectDelay = 1000;

    sse.onopen = function() {
      // SSE is live — no need to poll
      stopPolling();
    };

    sse.onmessage = function(event) {
      try {
        var pending = JSON.parse(event.data);
        renderConfirm(pending);
      } catch(e) {}
    };

    sse.onerror = function() {
      if (sse.readyState === EventSource.CLOSED) {
        window._confirmSSE = null;
        // Resume polling while we're disconnected
        startPolling();
        scheduleReconnect();
      }
      // If CONNECTING, EventSource is auto-reconnecting — do nothing
    };
  }

  function scheduleReconnect() {
    if (window._confirmReconnectTimer) return;
    window._confirmReconnectTimer = setTimeout(function() {
      window._confirmReconnectTimer = null;
      connectSSE();
    }, reconnectDelay);
    reconnectDelay = Math.min(reconnectDelay * 2, 15000);
  }

  function init() {
    // Immediate check for anything pending right now
    pollPending();
    connectSSE();
    // Poll until SSE opens; onopen will stop polling, onerror will resume it.
    startPolling();
  }

  function showToast(msg) {
    var t = document.createElement('div');
    t.className = 'fixed bottom-4 right-4 z-50 px-3 py-2 bg-gray-800 text-white text-xs rounded shadow-lg';
    t.textContent = msg;
    document.body.appendChild(t);
    setTimeout(function() { t.remove(); }, 2500);
  }

  window.respondConfirm = function(id, action) {
    fetch('/confirm/' + id + '/' + action, { method: 'POST' })
      .then(function(r) {
        if (r.status === 404) {
          showToast('Already resolved by another tab');
        }
      })
      .catch(function() {});
    document.getElementById('confirm-modal').classList.add('hidden');
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
