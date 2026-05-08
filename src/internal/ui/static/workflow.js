// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

function addStep() {
  const list = document.getElementById('steps-list');
  const rows = list.querySelectorAll('.step-row');
  const idx = rows.length;

  const empty = document.getElementById('empty-state');
  if (empty) empty.remove();

  let options = '';
  const firstSelect = list.querySelector('.step-tool');
  if (firstSelect) options = firstSelect.innerHTML;

  const details = document.createElement('details');
  details.className = 'step-row border-b border-gray-100 last:border-b-0';
  details.dataset.index = idx;
  details.open = true;
  details.innerHTML = `
    <summary class="px-4 py-3 flex items-center gap-3 cursor-pointer hover:bg-gray-50 transition-colors">
      <span class="step-num text-[10px] font-bold text-gray-400 w-4 text-right shrink-0">${idx + 1}</span>
      <span class="text-sm font-mono text-gray-400 flex-1">new step</span>
      <span class="flex gap-0.5 shrink-0 ml-1">
        <button type="button" onclick="event.preventDefault(); moveStep(this, -1)" class="text-gray-300 hover:text-gray-600 text-[10px] px-0.5" title="Move up">▲</button>
        <button type="button" onclick="event.preventDefault(); moveStep(this, 1)" class="text-gray-300 hover:text-gray-600 text-[10px] px-0.5" title="Move down">▼</button>
      </span>
      <button type="button" onclick="event.preventDefault(); removeStep(this)" class="text-gray-300 hover:text-red-500 text-xs shrink-0">✕</button>
    </summary>
    <div class="px-4 pb-4 pt-1 pl-11 space-y-3 bg-gray-50/50">
      <div>
        <label class="block text-[10px] font-medium text-gray-400 uppercase mb-1">Tool</label>
        <select name="step_tool_${idx}" class="step-tool w-full px-2 py-1.5 text-sm font-mono border border-gray-200 rounded bg-white focus:outline-none focus:ring-1 focus:ring-indigo-200">${options}</select>
      </div>
      <div>
        <label class="block text-[10px] font-medium text-gray-400 uppercase mb-1">Store output as</label>
        <input type="text" name="step_store_${idx}" placeholder="variable name" class="step-store w-full px-2 py-1.5 text-sm font-mono border border-gray-200 rounded bg-white focus:outline-none focus:ring-1 focus:ring-indigo-200">
      </div>
      <div>
        <label class="block text-[10px] font-medium text-gray-400 uppercase mb-1">If (skip when false)</label>
        <input type="text" name="step_if_${idx}" placeholder="e.g. changes != ''" class="w-full px-2 py-1.5 text-sm font-mono border border-gray-200 rounded bg-white focus:outline-none focus:ring-1 focus:ring-indigo-200">
      </div>
      <div>
        <label class="block text-[10px] font-medium text-gray-400 uppercase mb-1">Require (stop when false)</label>
        <input type="text" name="step_require_${idx}" placeholder="e.g. contains(output, 'PASS')" class="w-full px-2 py-1.5 text-sm font-mono border border-gray-200 rounded bg-white focus:outline-none focus:ring-1 focus:ring-indigo-200">
      </div>
      <div>
        <label class="block text-[10px] font-medium text-gray-400 uppercase mb-1">Params</label>
        <div class="space-y-1 step-params" data-index="${idx}"></div>
        <button type="button" onclick="addParam(this)" class="mt-1 text-[10px] text-indigo-500 hover:text-indigo-700">+ param</button>
      </div>
    </div>
  `;
  list.appendChild(details);
}

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
}
