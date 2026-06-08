// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

function addParam(btn) {
  const stepRow = btn.closest('.step-row');
  const stepIdx = stepRow.dataset.index;
  const container = btn.previousElementSibling;
  const row = document.createElement('div');
  row.className = 'param-row flex gap-1';
  row.innerHTML = `
    <input type="text" name="step_param_key_${stepIdx}[]" placeholder="key" class="w-1/3 px-2 py-1 text-xs font-mono border border-gray-200 rounded bg-white">
    <input type="text" name="step_param_val_${stepIdx}[]" placeholder="value" class="flex-1 px-2 py-1 text-xs font-mono border border-gray-200 rounded bg-white">
    <button type="button" onclick="this.closest('.param-row').remove()" class="text-gray-300 hover:text-red-500 text-xs px-1">✕</button>
  `;
  container.appendChild(row);
}

// reindexWfParams renumbers every wf-param-row's input names so they
// run 0..N-1 with no gaps. Call after add/remove/reorder so the form's
// indexed wf_param_*_<i> names stay contiguous — the server-side parser
// stops at the first empty index, so a gap means later params get lost
// on save.
function reindexWfParams() {
  const list = document.getElementById('wf-params-list');
  if (!list) return;
  const rows = list.querySelectorAll('.wf-param-row');
  rows.forEach((r, i) => {
    r.querySelectorAll('[name^="wf_param_name_"]').forEach(el => el.name = 'wf_param_name_' + i);
    r.querySelectorAll('[name^="wf_param_type_"]').forEach(el => el.name = 'wf_param_type_' + i);
    r.querySelectorAll('[name^="wf_param_required_"]').forEach(el => el.name = 'wf_param_required_' + i);
    r.querySelectorAll('[name^="wf_param_default_"]').forEach(el => el.name = 'wf_param_default_' + i);
    r.querySelectorAll('[name^="wf_param_desc_"]').forEach(el => el.name = 'wf_param_desc_' + i);
    r.querySelectorAll('[name^="wf_param_enum_"]').forEach(el => el.name = 'wf_param_enum_' + i);
  });
}

function reindexSteps() {
  const list = document.getElementById('steps-list');
  const rows = list.querySelectorAll('.step-row');
  rows.forEach((r, i) => {
    r.dataset.index = i;
    const num = r.querySelector('.step-num');
    if (num) num.textContent = i + 1;
    r.querySelectorAll('[name^="step_tool_"]').forEach(el => el.name = 'step_tool_' + i);
    r.querySelectorAll('[name^="step_store_"]').forEach(el => el.name = 'step_store_' + i);
    r.querySelectorAll('[name^="step_if_"]').forEach(el => el.name = 'step_if_' + i);
    r.querySelectorAll('[name^="step_require_"]').forEach(el => el.name = 'step_require_' + i);
    r.querySelectorAll('[name^="step_param_key_"]').forEach(el => el.name = 'step_param_key_' + i + '[]');
    r.querySelectorAll('[name^="step_param_val_"]').forEach(el => el.name = 'step_param_val_' + i + '[]');
    const params = r.querySelector('.step-params');
    if (params) params.dataset.index = i;
  });
}

function moveStep(btn, direction) {
  const row = btn.closest('.step-row');
  const list = document.getElementById('steps-list');
  const rows = Array.from(list.querySelectorAll('.step-row'));
  const idx = rows.indexOf(row);
  const targetIdx = idx + direction;
  if (targetIdx < 0 || targetIdx >= rows.length) return;

  const other = rows[targetIdx];
  swapStepValues(row, other);
  updateStepSummary(row);
  updateStepSummary(other);
}

function swapStepValues(rowA, rowB) {
  const selA = rowA.querySelector('.step-tool');
  const selB = rowB.querySelector('.step-tool');
  if (selA && selB) {
    const tmp = selA.value;
    selA.value = selB.value;
    selB.value = tmp;
  }

  const inputsA = rowA.querySelectorAll('input[type="text"]');
  const inputsB = rowB.querySelectorAll('input[type="text"]');
  const coreInputsA = Array.from(inputsA).filter(el => !el.name.includes('param'));
  const coreInputsB = Array.from(inputsB).filter(el => !el.name.includes('param'));

  for (let i = 0; i < Math.min(coreInputsA.length, coreInputsB.length); i++) {
    const tmp = coreInputsA[i].value;
    coreInputsA[i].value = coreInputsB[i].value;
    coreInputsB[i].value = tmp;
  }

  const paramsA = rowA.querySelector('.step-params');
  const paramsB = rowB.querySelector('.step-params');
  if (paramsA && paramsB) {
    const tmpHTML = paramsA.innerHTML;
    paramsA.innerHTML = paramsB.innerHTML;
    paramsB.innerHTML = tmpHTML;
  }

  reindexSteps();
}

function updateStepSummary(row) {
  const sel = row.querySelector('.step-tool');
  const summaryTool = row.querySelector('summary .font-mono');
  if (sel && summaryTool) {
    summaryTool.textContent = sel.value || 'new step';
  }
  const storeInput = row.querySelector('input[name^="step_store_"]');
  const storeSpan = row.querySelector('summary .text-indigo-600');
  const arrowSpan = storeSpan ? storeSpan.previousElementSibling : null;
  if (storeInput && storeSpan) {
    if (storeInput.value) {
      storeSpan.textContent = storeInput.value;
      storeSpan.classList.remove('hidden');
      if (arrowSpan) arrowSpan.classList.remove('hidden');
    } else {
      storeSpan.classList.add('hidden');
      if (arrowSpan) arrowSpan.classList.add('hidden');
    }
  }
}

function removeStep(btn) {
  const row = btn.closest('.step-row');
  row.remove();
  reindexSteps();
  if (document.querySelectorAll('.step-row').length === 0) {
    document.getElementById('steps-list').innerHTML = '<div class="px-4 py-8 text-center text-gray-400 text-sm" id="empty-state">No steps. Click "+ Add" to start.</div>';
  }
  const area = document.getElementById('tool-edit-area');
  if (area) area.dataset.dirty = '1';
}
