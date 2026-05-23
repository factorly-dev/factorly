// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

// Minimal SSE bootstrap. Sole job: keep window._activitySSE alive so
// workflow_edit.html's "Try It" step-progress poller has something
// to attach to. The dashboard's live feed owns its own EventSource;
// this file is only here because we removed the activity drawer
// (and with it activity.js) but workflow_edit still depends on the
// global handle.
(function() {
  function init() {
    if (window._activitySSE && window._activitySSE.readyState === EventSource.CLOSED) {
      window._activitySSE = null;
    }
    if (window._activitySSE) return;
    window._activitySSE = new EventSource('/activity/stream');
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
