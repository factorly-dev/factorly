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

function _renderCallEvent(data) {
  var feed = document.getElementById('activity-feed');
  var empty = document.getElementById('activity-empty');
  if (empty) empty.remove();

  var icon = '✓';
  var iconClass = 'bg-green-100 text-green-600';
  if (data.status === 'blocked') {
    icon = '✗';
    iconClass = 'bg-red-100 text-red-600';
  } else if (data.status === 'error') {
    icon = '!';
    iconClass = 'bg-amber-100 text-amber-600';
  }

  var shadow = '';
  if (data.shadow && data.shadow !== 'allowed') {
    shadow = '<span class="px-1 py-0.5 text-[9px] rounded bg-amber-50 text-amber-700">' + escapeHtml(data.shadow) + '</span>';
  }

  var paramsHtml = '';
  if (data.params && Object.keys(data.params).length > 0) {
    paramsHtml = '<div class="mb-2"><div class="text-[9px] text-gray-400 uppercase mb-1">Params</div>';
    for (var k in data.params) {
      paramsHtml += '<div class="mb-1"><span class="text-[9px] text-gray-500">' + escapeHtml(k) + '</span><pre class="text-[9px] font-mono bg-gray-50 text-gray-700 px-1.5 py-1 rounded mt-0.5 whitespace-pre-wrap break-all max-h-24 overflow-y-auto">' + escapeHtml(data.params[k]) + '</pre></div>';
    }
    paramsHtml += '</div>';
  }

  var detailHtml = '';
  if (data.error) {
    detailHtml += '<div class="bg-red-50 rounded p-1.5 mb-1"><pre class="text-red-700 text-[9px] font-mono whitespace-pre-wrap">' + escapeHtml(data.error) + '</pre></div>';
  }
  if (data.output) {
    detailHtml += '<div class="bg-gray-900 rounded p-1.5"><pre class="text-green-300 text-[9px] font-mono whitespace-pre-wrap max-h-32 overflow-y-auto">' + escapeHtml(data.output) + '</pre></div>';
  }

  var row = document.createElement('details');
  row.className = 'border-b border-gray-100';
  row.dataset.tool = data.tool;
  row.innerHTML =
    '<summary class="px-4 py-2 cursor-pointer hover:bg-gray-50 transition-colors flex items-center justify-between text-xs">' +
      '<div class="flex items-center gap-1.5">' +
        '<span class="inline-flex items-center justify-center w-4 h-4 rounded-full ' + iconClass + ' text-[8px] font-bold">' + icon + '</span>' +
        '<span class="font-mono text-[11px]">' + escapeHtml(data.tool) + '</span>' +
        shadow +
      '</div>' +
      '<div class="flex items-center gap-2 text-[10px] text-gray-400 shrink-0">' +
        '<span class="font-mono">' + data.duration_ms + 'ms</span>' +
        '<span>' + data.timestamp + '</span>' +
      '</div>' +
    '</summary>' +
    '<div class="px-4 pb-2 pt-1 pl-9">' +
      '<div class="workflow-steps"></div>' +
      paramsHtml +
      detailHtml +
      (!paramsHtml && !detailHtml ? '<span class="text-[9px] text-gray-300">No details</span>' : '') +
    '</div>';

  feed.insertBefore(row, feed.firstChild);

  activityCount++;
  document.getElementById('drawer-count').textContent = activityCount + (activityCount === 1 ? ' call' : ' calls');
  document.getElementById('activity-badge').textContent = 'Activity (' + activityCount + ')';

  var dot = document.getElementById('activity-dot');
  dot.classList.remove('bg-green-500');
  dot.classList.add('bg-indigo-500');
  setTimeout(function() { dot.classList.remove('bg-indigo-500'); dot.classList.add('bg-green-500'); }, 300);
}

function _renderStepEvent(data) {
  var feed = document.getElementById('activity-feed');

  var icon = '…';
  var iconClass = 'bg-gray-100 text-gray-500';
  if (data.status === 'completed') {
    icon = '✓';
    iconClass = 'bg-green-100 text-green-600';
  } else if (data.status === 'failed') {
    icon = '✗';
    iconClass = 'bg-red-100 text-red-600';
  } else if (data.status === 'skipped' || data.status === 'stopped') {
    icon = '–';
    iconClass = 'bg-gray-100 text-gray-400';
  } else if (data.status === 'running') {
    icon = '●';
    iconClass = 'bg-indigo-100 text-indigo-600';
  }

  var durHtml = data.duration_ms ? '<span class="font-mono text-gray-400">' + data.duration_ms + 'ms</span>' : '';
  var errorHtml = data.error ? '<div class="text-[9px] text-red-500 font-mono mt-0.5">' + escapeHtml(data.error) + '</div>' : '';

  var stepEl = document.createElement('div');
  stepEl.className = 'flex items-center gap-1.5 py-0.5 text-[10px]';
  stepEl.dataset.stepIndex = data.index;
  stepEl.dataset.runId = data.run_id;
  stepEl.innerHTML =
    '<span class="inline-flex items-center justify-center w-3 h-3 rounded-full ' + iconClass + ' text-[7px] font-bold">' + icon + '</span>' +
    '<span class="text-gray-500">' + data.index + '/' + data.total + '</span>' +
    '<span class="font-mono text-gray-700">' + escapeHtml(data.tool) + '</span>' +
    '<span class="text-gray-400">' + data.status + '</span>' +
    durHtml +
    errorHtml;

  // Find the parent workflow entry and append step inside it
  var workflowRow = feed.querySelector('details[data-tool="' + data.workflow + '"]');
  if (workflowRow) {
    var stepsContainer = workflowRow.querySelector('.workflow-steps');
    if (stepsContainer) {
      // Replace existing step with same index+runId, or append
      var existing = stepsContainer.querySelector('[data-step-index="' + data.index + '"][data-run-id="' + data.run_id + '"]');
      if (existing) {
        existing.replaceWith(stepEl);
      } else {
        stepsContainer.appendChild(stepEl);
      }
      return;
    }
  }

  // No parent workflow row yet — render standalone
  var standalone = document.createElement('div');
  standalone.className = 'px-4 py-1 border-b border-gray-50 bg-gray-50/50';
  standalone.appendChild(stepEl);
  feed.insertBefore(standalone, feed.firstChild);
}

// SSE connection (singleton — only create once across hx-boost navigations)
function _initActivitySSE() {
  if (window._activitySSE && window._activitySSE.readyState === EventSource.CLOSED) {
    window._activitySSE = null;
  }
  if (window._activitySSE) return;
  window._activitySSE = new EventSource('/activity/stream');

  window._activitySSE.addEventListener('call', function(event) {
    _renderCallEvent(JSON.parse(event.data));
  });

  window._activitySSE.addEventListener('workflow_step', function(event) {
    _renderStepEvent(JSON.parse(event.data));
  });
}
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', _initActivitySSE);
} else {
  _initActivitySSE();
}
