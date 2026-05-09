// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

// Activity drawer — use var to avoid redeclaration on hx-boost navigation
if (typeof activityOpen === 'undefined') {
  var activityOpen = false;
  var activityCount = 0;
}

function toggleActivity() {
  activityOpen = !activityOpen;
  document.getElementById('activity-drawer').classList.toggle('open', activityOpen);
}

function clearActivity() {
  document.getElementById('activity-feed').innerHTML = '<div id="activity-empty" class="px-4 py-12 text-center text-gray-400 text-xs">Waiting for agent activity...</div>';
  activityCount = 0;
  document.getElementById('drawer-count').textContent = '0 calls';
  document.getElementById('activity-badge').textContent = 'Activity';
}

// SSE connection (singleton — only create once across hx-boost navigations)
// Use DOMContentLoaded to ensure page is ready; for hx-boost navigations
// the script re-executes after swap so the guard prevents duplicates.
function _initActivitySSE() {
  // Reconnect if previous connection was closed (e.g., by auth redirect)
  if (window._activitySSE && window._activitySSE.readyState === EventSource.CLOSED) {
    window._activitySSE = null;
  }
  if (window._activitySSE) return;
  window._activitySSE = new EventSource('/activity/stream');

  window._activitySSE.onmessage = function(event) {
    const data = JSON.parse(event.data);
    const feed = document.getElementById('activity-feed');

    // Remove empty state
    const empty = document.getElementById('activity-empty');
    if (empty) empty.remove();

    let icon = '✓';
    let iconClass = 'bg-green-100 text-green-600';
    if (data.status === 'blocked') {
      icon = '✗';
      iconClass = 'bg-red-100 text-red-600';
    } else if (data.status === 'error') {
      icon = '!';
      iconClass = 'bg-amber-100 text-amber-600';
    }

    let shadow = '';
    if (data.shadow && data.shadow !== 'allowed') {
      shadow = `<span class="px-1 py-0.5 text-[9px] rounded bg-amber-50 text-amber-700">${escapeHtml(data.shadow)}</span>`;
    }

    let paramsHtml = '';
    if (data.params && Object.keys(data.params).length > 0) {
      paramsHtml = '<div class="mb-2"><div class="text-[9px] text-gray-400 uppercase mb-1">Params</div>';
      for (const [k, v] of Object.entries(data.params)) {
        paramsHtml += `<div class="mb-1"><span class="text-[9px] text-gray-500">${escapeHtml(k)}</span><pre class="text-[9px] font-mono bg-gray-50 text-gray-700 px-1.5 py-1 rounded mt-0.5 whitespace-pre-wrap break-all max-h-24 overflow-y-auto">${escapeHtml(v)}</pre></div>`;
      }
      paramsHtml += '</div>';
    }

    let detailHtml = '';
    if (data.error) {
      detailHtml += `<div class="bg-red-50 rounded p-1.5 mb-1"><pre class="text-red-700 text-[9px] font-mono whitespace-pre-wrap">${escapeHtml(data.error)}</pre></div>`;
    }
    if (data.output) {
      detailHtml += `<div class="bg-gray-900 rounded p-1.5"><pre class="text-green-300 text-[9px] font-mono whitespace-pre-wrap max-h-32 overflow-y-auto">${escapeHtml(data.output)}</pre></div>`;
    }

    const row = document.createElement('details');
    row.className = 'border-b border-gray-100';
    row.innerHTML = `
      <summary class="px-4 py-2 cursor-pointer hover:bg-gray-50 transition-colors flex items-center justify-between text-xs">
        <div class="flex items-center gap-1.5">
          <span class="inline-flex items-center justify-center w-4 h-4 rounded-full ${iconClass} text-[8px] font-bold">${icon}</span>
          <span class="font-mono text-[11px]">${escapeHtml(data.tool)}</span>
          ${shadow}
        </div>
        <div class="flex items-center gap-2 text-[10px] text-gray-400 shrink-0">
          <span class="font-mono">${data.duration_ms}ms</span>
          <span>${data.timestamp}</span>
        </div>
      </summary>
      <div class="px-4 pb-2 pt-1 pl-9">
        ${paramsHtml}
        ${detailHtml}
        ${!paramsHtml && !detailHtml ? '<span class="text-[9px] text-gray-300">No details</span>' : ''}
      </div>
    `;

    feed.insertBefore(row, feed.firstChild);

    activityCount++;
    document.getElementById('drawer-count').textContent = activityCount + (activityCount === 1 ? ' call' : ' calls');
    document.getElementById('activity-badge').textContent = 'Activity (' + activityCount + ')';

    // Flash the dot
    const dot = document.getElementById('activity-dot');
    dot.classList.remove('bg-green-500');
    dot.classList.add('bg-indigo-500');
    setTimeout(() => { dot.classList.remove('bg-indigo-500'); dot.classList.add('bg-green-500'); }, 300);
  };
}
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', _initActivitySSE);
} else {
  _initActivitySSE();
}
