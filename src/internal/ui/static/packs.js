// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

// Pack import modal — two-stage flow:
//   1. User enters a source (GitHub shorthand / URL / local path) and clicks
//      Preview. We POST /packs/preview, which returns a structured
//      InstallResult describing what would be added, conflicts, missing
//      dependencies, vault keys needed, and an already-installed flag.
//   2. User reviews the preview, fills in any vault key values, and clicks
//      Install. We POST /packs/install with the vault values; the server
//      writes the pack file, stores the vault values, and reloads config.
//
// All interactions use data-pack-action attributes routed through a single
// delegated click handler, so the template never needs to inject names into
// onclick="..." strings — eliminates an XSS-shaped risk and a stale state
// class of bugs from per-element handlers.
//
// Guard against hx-boost re-execution: bind once.

if (typeof window._packsInit === 'undefined') {
  window._packsInit = true;

  document.addEventListener('click', function(e) {
    var btn = e.target.closest('[data-pack-action]');
    if (!btn) return;
    var action = btn.getAttribute('data-pack-action');
    switch (action) {
      case 'open-import':
        openModal();
        break;
      case 'close-import':
        closeModal();
        break;
      case 'preview':
        runPreview();
        break;
      case 'reset-preview':
        resetPreview();
        break;
      case 'install':
        runInstall(btn.getAttribute('data-pack-source'));
        break;
      case 'uninstall':
        runUninstall(btn.getAttribute('data-pack-name'));
        break;
    }
  });

  // Modal keyboard shortcuts: Enter submits stage 1, ESC closes the modal.
  document.addEventListener('keydown', function(e) {
    var modal = document.getElementById('pack-import-modal');
    if (!modal || modal.classList.contains('hidden')) return;
    if (e.key === 'Escape') {
      closeModal();
    } else if (e.key === 'Enter') {
      var input = document.getElementById('pack-import-source-input');
      if (input && document.activeElement === input) {
        e.preventDefault();
        runPreview();
      }
    }
  });

  function openModal() {
    var m = document.getElementById('pack-import-modal');
    if (!m) return;
    m.classList.remove('hidden');
    document.getElementById('pack-import-source-stage').classList.remove('hidden');
    document.getElementById('pack-import-preview-stage').classList.add('hidden');
    document.getElementById('pack-import-preview-stage').innerHTML = '';
    hideError();
    var input = document.getElementById('pack-import-source-input');
    if (input) {
      input.value = '';
      setTimeout(function() { input.focus(); }, 50);
    }
  }

  function closeModal() {
    var m = document.getElementById('pack-import-modal');
    if (m) m.classList.add('hidden');
  }

  function runPreview() {
    var source = (document.getElementById('pack-import-source-input').value || '').trim();
    if (!source) {
      showError('source is required');
      return;
    }
    hideError();
    fetch('/packs/preview', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ source: source })
    })
      .then(function(r) { return r.json().catch(function() { return { error: 'HTTP ' + r.status }; }); })
      .then(function(data) {
        if (data.error && !data.result) {
          showError(data.error);
          return;
        }
        renderPreview(data.result, data.error, source);
      })
      .catch(function(err) { showError(err.message || String(err)); });
  }

  function runInstall(source) {
    if (!source) {
      showError('install source missing');
      return;
    }
    var vaultInputs = document.querySelectorAll('#pack-import-preview-stage input[data-vault-key]');
    var vaultValues = {};
    vaultInputs.forEach(function(input) {
      var key = input.getAttribute('data-vault-key');
      var val = (input.value || '').trim();
      if (val) vaultValues[key] = val;
    });

    hideError();
    fetch('/packs/install', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ source: source, vault_values: vaultValues })
    })
      .then(function(r) { return r.json().catch(function() { return { error: 'HTTP ' + r.status }; }); })
      .then(function(data) {
        if (data.error) {
          showError(data.error);
          return;
        }
        closeModal();
        window.location.reload();
      })
      .catch(function(err) { showError(err.message || String(err)); });
  }

  function runUninstall(name) {
    if (!name) return;
    if (!confirm("Uninstall pack '" + name + "'?")) return;
    fetch('/packs/' + encodeURIComponent(name), { method: 'DELETE' })
      .then(function(r) {
        if (r.ok) {
          window.location.reload();
        } else {
          return r.text().then(function(t) { alert('Uninstall failed: ' + t); });
        }
      })
      .catch(function(err) { alert('Uninstall failed: ' + (err.message || err)); });
  }

  function renderPreview(result, errorMsg, source) {
    document.getElementById('pack-import-source-stage').classList.add('hidden');
    var stage = document.getElementById('pack-import-preview-stage');
    stage.classList.remove('hidden');

    var header = (result && result.header) || {};
    var html = '';

    var title = header.name || '(unnamed pack)';
    var subtitle = header.version ? (title + ' ' + header.version) : title;
    html += '<div class="mb-3">';
    html += '<div class="text-base font-medium text-gray-900">' + escapeHtml(subtitle) + '</div>';
    if (header.description) {
      html += '<div class="text-xs text-gray-600 mt-0.5">' + escapeHtml(header.description) + '</div>';
    }
    if (header.author || header.license || header.homepage) {
      html += '<div class="text-[10px] text-gray-400 mt-1">';
      if (header.author) html += escapeHtml(header.author);
      if (header.license) html += (header.author ? ' • ' : '') + escapeHtml(header.license);
      if (header.homepage) html += ' • <a href="' + escapeHtml(header.homepage) + '" target="_blank" rel="noopener" class="text-indigo-500 hover:text-indigo-700">' + escapeHtml(header.homepage) + '</a>';
      html += '</div>';
    }
    html += '</div>';

    if (result && result.already_installed) {
      html += '<div class="mb-3 bg-amber-50 border border-amber-200 rounded p-2 text-xs text-amber-800">';
      html += 'This pack is already installed. Uninstall it first to reinstall.';
      html += '</div>';
    }

    html += renderListSection('Tools', (result && result.tools_added) || []);
    html += renderListSection('Workflows', (result && result.workflows_added) || []);
    html += renderListSection('OAuth Providers', (result && result.providers_added) || []);
    html += renderListSection('Vault Backends', (result && result.vault_backends) || []);

    if (result && result.conflicts && result.conflicts.length > 0) {
      html += '<div class="mb-3"><div class="text-[10px] font-medium text-red-600 uppercase tracking-wide mb-1">Conflicts</div>';
      html += '<ul class="space-y-0.5">';
      result.conflicts.forEach(function(c) {
        html += '<li class="text-xs text-red-700">✗ ' + escapeHtml(c.kind) + ' "' + escapeHtml(c.name) + '" already defined</li>';
      });
      html += '</ul></div>';
    }

    if (result && result.requires_missing && result.requires_missing.length > 0) {
      html += '<div class="mb-3"><div class="text-[10px] font-medium text-red-600 uppercase tracking-wide mb-1">Missing Dependencies</div>';
      html += '<ul class="space-y-0.5">';
      result.requires_missing.forEach(function(r) {
        html += '<li class="text-xs text-red-700">✗ ' + escapeHtml(r.kind) + ' "' + escapeHtml(r.name) + '" not installed</li>';
      });
      html += '</ul></div>';
    }

    if (result && result.vault_keys_missing && result.vault_keys_missing.length > 0) {
      html += '<div class="mb-3"><div class="text-[10px] font-medium text-gray-500 uppercase tracking-wide mb-1">Vault Keys Required</div>';
      html += '<div class="text-[10px] text-gray-400 mb-2">Provide values now (or leave blank to set later with <code>factorly vault set</code>).</div>';
      result.vault_keys_missing.forEach(function(key) {
        html += '<div class="mb-1.5 flex items-center gap-2">';
        html += '<label class="font-mono text-[10px] text-gray-600 w-40 shrink-0">' + escapeHtml(key) + '</label>';
        html += '<input type="password" data-vault-key="' + escapeHtml(key) + '" class="flex-1 px-2 py-1 text-xs font-mono border border-gray-200 rounded focus:outline-none focus:ring-1 focus:ring-indigo-200">';
        html += '</div>';
      });
      html += '</div>';
    }

    if (errorMsg) {
      html += '<div class="mb-3 bg-amber-50 border border-amber-200 rounded p-2 text-xs text-amber-800">' + escapeHtml(errorMsg) + '</div>';
    }

    var blocked = !result ||
                  (result.conflicts && result.conflicts.length > 0) ||
                  (result.requires_missing && result.requires_missing.length > 0) ||
                  result.already_installed;
    html += '<div class="flex justify-end gap-2 mt-4">';
    html += '<button data-pack-action="reset-preview" class="px-3 py-1.5 text-xs text-gray-600 border border-gray-200 rounded hover:bg-gray-50">Back</button>';
    html += '<button data-pack-action="close-import" class="px-3 py-1.5 text-xs text-gray-600 border border-gray-200 rounded hover:bg-gray-50">Cancel</button>';
    if (blocked) {
      html += '<button disabled class="px-4 py-1.5 text-xs bg-gray-300 text-white rounded cursor-not-allowed">Install</button>';
    } else {
      html += '<button data-pack-action="install" data-pack-source="' + escapeHtml(source) + '" class="px-4 py-1.5 text-xs bg-indigo-600 text-white rounded hover:bg-indigo-700">Install</button>';
    }
    html += '</div>';

    stage.innerHTML = html;
  }

  function resetPreview() {
    // Keep the previous source in the input so a failed preview is easy to
    // retry with a tweak.
    document.getElementById('pack-import-source-stage').classList.remove('hidden');
    document.getElementById('pack-import-preview-stage').classList.add('hidden');
    document.getElementById('pack-import-preview-stage').innerHTML = '';
    hideError();
    var input = document.getElementById('pack-import-source-input');
    if (input) setTimeout(function() { input.focus(); }, 50);
  }

  function renderListSection(title, items) {
    if (!items || items.length === 0) return '';
    var html = '<div class="mb-3"><div class="text-[10px] font-medium text-gray-500 uppercase tracking-wide mb-1">' + escapeHtml(title) + ' (' + items.length + ')</div>';
    html += '<ul class="space-y-0.5">';
    items.forEach(function(name) {
      html += '<li class="text-xs text-gray-700"><span class="text-green-500">+</span> <span class="font-mono">' + escapeHtml(name) + '</span></li>';
    });
    html += '</ul></div>';
    return html;
  }

  function showError(msg) {
    var el = document.getElementById('pack-import-error');
    if (!el) return;
    el.textContent = msg;
    el.classList.remove('hidden');
  }

  function hideError() {
    var el = document.getElementById('pack-import-error');
    if (el) el.classList.add('hidden');
  }
}
