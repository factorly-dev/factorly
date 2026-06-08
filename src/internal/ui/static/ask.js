// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

// Ask modal — SSE stream for factorly.ask prompts with polling fallback.
// Mirrors confirm.js's broker/SSE/render/respond pattern; the only
// structural difference is the modal body renders a single typed input
// instead of approve/deny buttons, and submission sends the user's
// answer (or a cancel signal) to the broker.
(function() {
  if (window._askInit) return;
  window._askInit = true;
  window._askSSE = null;
  window._askPollTimer = null;
  window._askReconnectTimer = null;
  window._askCurrentId = null;
  var reconnectDelay = 1000;

  function escapeAttr(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  // Render the input widget for a question. Mapping mirrors
  // try_panel.html so an ask form feels familiar to anyone who's
  // used the per-tool Try-It panel.
  function inputHtml(req) {
    var name = 'answer';
    var def = req.default == null ? '' : String(req.default);
    var hasEnum = Array.isArray(req.enum) && req.enum.length > 0;
    if (hasEnum) {
      var opts = '';
      for (var i = 0; i < req.enum.length; i++) {
        var v = req.enum[i];
        var sel = (v === def) ? ' selected' : '';
        opts += '<option value="' + escapeAttr(v) + '"' + sel + '>' + escapeHtml(v) + '</option>';
      }
      return '<select name="' + name + '" autofocus class="w-full px-3 py-2 border border-gray-300 rounded text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-200">' + opts + '</select>';
    }
    if (req.type === 'boolean') {
      var checked = (def === 'true' || def === '1' || def === 'yes') ? ' checked' : '';
      return '<label class="inline-flex items-center gap-2 text-sm"><input type="checkbox" name="' + name + '" value="true"' + checked + ' class="rounded border-gray-300 text-indigo-600 focus:ring-indigo-200"><span class="text-gray-700">Yes</span></label>';
    }
    if (req.type === 'text') {
      return '<textarea name="' + name + '" rows="4" autofocus class="w-full px-3 py-2 border border-gray-300 rounded text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-200">' + escapeHtml(def) + '</textarea>';
    }
    if (req.type === 'integer' || req.type === 'number') {
      var attrs = req.type === 'integer' ? ' step="1"' : '';
      return '<input type="number" name="' + name + '" value="' + escapeAttr(def) + '" autofocus' + attrs + ' class="w-full px-3 py-2 border border-gray-300 rounded text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-200">';
    }
    // Fallback — string / json / unknown.
    return '<input type="text" name="' + name + '" value="' + escapeAttr(def) + '" autofocus class="w-full px-3 py-2 border border-gray-300 rounded text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-200">';
  }

  function renderAsk(pending) {
    var modal = document.getElementById('ask-modal');
    if (!modal) return;
    if (!pending || pending.length === 0) {
      modal.classList.add('hidden');
      window._askCurrentId = null;
      document.getElementById('ask-content').innerHTML = '';
      return;
    }
    var req = pending[0];
    // Skip re-rendering if we're already showing this request —
    // otherwise an SSE redelivery would clobber the user's
    // in-progress typing.
    if (window._askCurrentId === req.id) return;
    window._askCurrentId = req.id;

    var label = req.name || '';
    var requiredMark = req.required ? ' <span class="text-red-500">*</span>' : '';
    var helpHtml = req.description
      ? '<p class="text-sm text-gray-700 mb-3">' + escapeHtml(req.description) + '</p>'
      : '';
    var titleHtml = req.title
      ? escapeHtml(req.title)
      : 'Input needed';

    document.getElementById('ask-content').innerHTML =
      '<div class="flex items-center gap-2 mb-2">' +
        '<span class="inline-flex items-center justify-center w-6 h-6 rounded-full bg-indigo-100 text-indigo-600 text-xs font-bold">?</span>' +
        '<span class="font-medium text-sm">' + titleHtml + '</span>' +
      '</div>' +
      helpHtml +
      '<form id="ask-form" onsubmit="return submitAsk(event, \'' + req.id + '\')">' +
        '<label class="block text-xs font-mono text-gray-500 mb-1">' + escapeHtml(label) + requiredMark + '</label>' +
        inputHtml(req) +
        '<div id="ask-error" class="text-xs text-red-600 mt-2 hidden"></div>' +
        '<div class="flex gap-2 mt-4">' +
          '<button type="submit" class="px-4 py-2 bg-indigo-600 text-white text-sm rounded hover:bg-indigo-700">Submit</button>' +
          '<button type="button" onclick="cancelAsk(\'' + req.id + '\')" class="px-4 py-2 bg-gray-50 text-gray-700 text-sm rounded border border-gray-200 hover:bg-gray-100">Cancel</button>' +
        '</div>' +
      '</form>';
    modal.classList.remove('hidden');
    // Focus the first input. autofocus alone doesn't always work
    // when content is injected mid-page.
    var first = modal.querySelector('input, textarea, select');
    if (first && typeof first.focus === 'function') first.focus();
  }

  function pollPending() {
    fetch('/ask/pending')
      .then(function(r) { return r.ok ? r.json() : []; })
      .then(renderAsk)
      .catch(function() {});
  }

  function startPolling() {
    if (window._askPollTimer) return;
    window._askPollTimer = setInterval(pollPending, 2000);
  }

  function stopPolling() {
    if (window._askPollTimer) {
      clearInterval(window._askPollTimer);
      window._askPollTimer = null;
    }
  }

  function connectSSE() {
    if (window._askSSE && window._askSSE.readyState !== EventSource.CLOSED) return;
    var sse = new EventSource('/ask/stream');
    window._askSSE = sse;
    reconnectDelay = 1000;

    sse.onopen = function() { stopPolling(); };
    sse.onmessage = function(event) {
      try {
        var pending = JSON.parse(event.data);
        renderAsk(pending);
      } catch(e) {}
    };
    sse.onerror = function() {
      if (sse.readyState === EventSource.CLOSED) {
        window._askSSE = null;
        startPolling();
        scheduleReconnect();
      }
    };
  }

  function scheduleReconnect() {
    if (window._askReconnectTimer) return;
    window._askReconnectTimer = setTimeout(function() {
      window._askReconnectTimer = null;
      connectSSE();
    }, reconnectDelay);
    reconnectDelay = Math.min(reconnectDelay * 2, 15000);
  }

  function init() {
    pollPending();
    connectSSE();
    startPolling();
  }

  function showError(msg) {
    var box = document.getElementById('ask-error');
    if (!box) return;
    box.textContent = msg;
    box.classList.remove('hidden');
  }

  window.submitAsk = function(event, id) {
    event.preventDefault();
    var form = event.target;
    var input = form.querySelector('[name="answer"]');
    var value = '';
    if (input) {
      if (input.type === 'checkbox') {
        value = input.checked ? 'true' : 'false';
      } else {
        value = input.value;
      }
    }
    var body = new URLSearchParams();
    body.set('action', 'submit');
    body.set('answer', value);
    fetch('/ask/' + id, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: body.toString(),
    }).then(function(r) {
      if (r.status === 400) {
        // Validation rejected — keep the modal open and show the
        // server's error message. The broker did NOT resolve the
        // request, so the user can correct and resubmit.
        return r.text().then(showError);
      }
      if (!r.ok && r.status !== 204) {
        showError('Submit failed (' + r.status + ')');
        return;
      }
      // 204 — broker resolved. SSE will clear the modal on next push.
      window._askCurrentId = null;
    }).catch(function() {
      showError('Submit failed (network error)');
    });
    return false;
  };

  window.cancelAsk = function(id) {
    var body = new URLSearchParams();
    body.set('action', 'cancel');
    fetch('/ask/' + id, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: body.toString(),
    }).catch(function() {});
    window._askCurrentId = null;
    document.getElementById('ask-modal').classList.add('hidden');
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
