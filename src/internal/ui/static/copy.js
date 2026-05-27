// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

// copyText — one-click copy for response containers.
//
// Usage:
//   <div class="relative">
//     <button onclick="copyText(this, 'pre')" ...>icon</button>
//     <pre>...content to copy...</pre>
//   </div>
//
// The button's nearest .relative ancestor scopes the lookup. The
// sourceSelector picks which child element's textContent to copy.
// On success the button's innerHTML is briefly swapped for a
// checkmark SVG, then restored after 1.5s.
(function() {
  var CHECK_SVG = '<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>';

  window.copyText = function(btn, sourceSelector) {
    var scope = btn.closest('.relative') || btn.parentElement;
    if (!scope) return;
    var src = scope.querySelector(sourceSelector);
    if (!src) return;
    var text = src.textContent || '';
    navigator.clipboard.writeText(text).then(function() {
      var orig = btn.innerHTML;
      btn.innerHTML = CHECK_SVG;
      setTimeout(function() { btn.innerHTML = orig; }, 1500);
    }).catch(function() {
      // Clipboard write can fail under restricted contexts (HTTP,
      // sandboxed iframes). Surface visibly so the user doesn't
      // silently get nothing on the clipboard.
      var orig = btn.innerHTML;
      btn.innerHTML = '!';
      setTimeout(function() { btn.innerHTML = orig; }, 1500);
    });
  };
})();
