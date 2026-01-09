// Zcash Web Wallet - SwapBundle Builder (NU7)

import { getWasm } from "./wasm.js";

let swapGroupCounter = 0;
let swapActionCounter = 0;
let swapBurnCounter = 0;

export function initSwapBundleUI() {
  const form = document.getElementById("swapBundleForm");
  const addGroupBtn = document.getElementById("swapAddGroupBtn");
  const copyHexBtn = document.getElementById("copySwapBundleHexBtn");

  if (!form) return;

  if (addGroupBtn) {
    addGroupBtn.addEventListener("click", () => addSwapGroupRow());
  }

  if (copyHexBtn) {
    copyHexBtn.addEventListener("click", copySwapBundleHex);
  }

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    buildSwapBundle();
  });

  addSwapGroupRow();
}

function addSwapGroupRow() {
  const groupsContainer = document.getElementById("swapGroups");
  if (!groupsContainer) return;

  const groupId = swapGroupCounter++;
  const group = document.createElement("div");
  group.className = "card border-secondary mb-3 swap-group";
  group.dataset.groupId = String(groupId);

  group.innerHTML = `
    <div class="card-header d-flex justify-content-between align-items-center">
      <h6 class="mb-0 fw-semibold">Action Group ${groupId + 1}</h6>
      <button type="button" class="btn btn-sm btn-outline-danger swap-group-remove">
        Remove Group
      </button>
    </div>
    <div class="card-body">
      <div class="row g-2">
        <div class="col-md-3">
          <label class="form-label">Flags (u8)</label>
          <input type="text" class="form-control swap-group-flags" value="0" />
        </div>
        <div class="col-md-5">
          <label class="form-label">Anchor (hex)</label>
          <input type="text" class="form-control mono swap-group-anchor" placeholder="32-byte hex" />
        </div>
        <div class="col-md-4">
          <label class="form-label">Expiry Height</label>
          <input type="text" class="form-control swap-group-expiry" value="0" readonly />
        </div>
      </div>

      <div class="mt-3">
        <label class="form-label fw-semibold">Actions</label>
        <div class="swap-actions"></div>
        <button type="button" class="btn btn-sm btn-outline-secondary swap-add-action">
          Add Action
        </button>
        <div class="form-text">
          enc_ciphertext must be 612 bytes; out_ciphertext must be 80 bytes.
        </div>
      </div>

      <div class="mt-3">
        <label class="form-label fw-semibold">Burns (optional)</label>
        <div class="swap-burns"></div>
        <button type="button" class="btn btn-sm btn-outline-secondary swap-add-burn">
          Add Burn
        </button>
      </div>

      <div class="mt-3">
        <label class="form-label fw-semibold">Proof (hex)</label>
        <textarea class="form-control mono small swap-group-proof" rows="3" placeholder="Proof hex"></textarea>
      </div>
    </div>
  `;

  const removeBtn = group.querySelector(".swap-group-remove");
  if (removeBtn) {
    removeBtn.addEventListener("click", () => {
      group.remove();
    });
  }

  const addActionBtn = group.querySelector(".swap-add-action");
  if (addActionBtn) {
    addActionBtn.addEventListener("click", () => addSwapActionRow(group));
  }

  const addBurnBtn = group.querySelector(".swap-add-burn");
  if (addBurnBtn) {
    addBurnBtn.addEventListener("click", () => addSwapBurnRow(group));
  }

  groupsContainer.appendChild(group);
  addSwapActionRow(group);
}

function addSwapActionRow(groupEl) {
  const actionsContainer = groupEl.querySelector(".swap-actions");
  if (!actionsContainer) return;

  const actionId = swapActionCounter++;
  const row = document.createElement("div");
  row.className = "border rounded p-2 mb-2 swap-action-row";
  row.dataset.actionId = String(actionId);

  row.innerHTML = `
    <div class="row g-2">
      <div class="col-md-6">
        <label class="form-label">cv_net (hex)</label>
        <input type="text" class="form-control mono swap-action-cv-net" placeholder="32-byte hex" />
      </div>
      <div class="col-md-6">
        <label class="form-label">Nullifier (hex)</label>
        <input type="text" class="form-control mono swap-action-nullifier" placeholder="32-byte hex" />
      </div>
      <div class="col-md-6">
        <label class="form-label">rk (hex)</label>
        <input type="text" class="form-control mono swap-action-rk" placeholder="32-byte hex" />
      </div>
      <div class="col-md-6">
        <label class="form-label">cmx (hex)</label>
        <input type="text" class="form-control mono swap-action-cmx" placeholder="32-byte hex" />
      </div>
      <div class="col-12">
        <label class="form-label">epk (hex)</label>
        <input type="text" class="form-control mono swap-action-epk" placeholder="32-byte hex" />
      </div>
      <div class="col-12">
        <label class="form-label">enc_ciphertext (hex)</label>
        <textarea class="form-control mono small swap-action-enc" rows="2" placeholder="612-byte hex"></textarea>
      </div>
      <div class="col-12">
        <label class="form-label">out_ciphertext (hex)</label>
        <textarea class="form-control mono small swap-action-out" rows="2" placeholder="80-byte hex"></textarea>
      </div>
      <div class="col-12">
        <label class="form-label">Spend Auth Signature (hex)</label>
        <input type="text" class="form-control mono swap-action-sig" placeholder="64-byte hex" />
      </div>
      <div class="col-12 text-end">
        <button type="button" class="btn btn-sm btn-outline-danger swap-action-remove">
          Remove Action
        </button>
      </div>
    </div>
  `;

  const removeBtn = row.querySelector(".swap-action-remove");
  if (removeBtn) {
    removeBtn.addEventListener("click", () => {
      row.remove();
    });
  }

  actionsContainer.appendChild(row);
}

function addSwapBurnRow(groupEl) {
  const burnsContainer = groupEl.querySelector(".swap-burns");
  if (!burnsContainer) return;

  const burnId = swapBurnCounter++;
  const row = document.createElement("div");
  row.className = "border rounded p-2 mb-2 swap-burn-row";
  row.dataset.burnId = String(burnId);

  row.innerHTML = `
    <div class="row g-2 align-items-end">
      <div class="col-md-8">
        <label class="form-label">Asset (hex)</label>
        <input type="text" class="form-control mono swap-burn-asset" placeholder="32-byte hex" />
      </div>
      <div class="col-md-4">
        <label class="form-label">Amount (u64)</label>
        <input type="text" class="form-control swap-burn-amount" placeholder="0" />
      </div>
      <div class="col-12 text-end">
        <button type="button" class="btn btn-sm btn-outline-danger swap-burn-remove">
          Remove Burn
        </button>
      </div>
    </div>
  `;

  const removeBtn = row.querySelector(".swap-burn-remove");
  if (removeBtn) {
    removeBtn.addEventListener("click", () => {
      row.remove();
    });
  }

  burnsContainer.appendChild(row);
}

function buildSwapBundle() {
  const wasm = getWasm();
  if (!wasm) {
    showSwapError("WASM module not loaded.");
    return;
  }

  const valueBalanceInput = document.getElementById("swapValueBalance");
  const bindingSigInput = document.getElementById("swapBindingSig");

  const valueBalance = valueBalanceInput?.value.trim() || "";
  const bindingSig = bindingSigInput?.value.trim() || "";

  if (!valueBalance) {
    showSwapError("Value balance is required.");
    return;
  }

  if (!bindingSig) {
    showSwapError("Binding signature is required.");
    return;
  }

  const groups = collectSwapGroups();
  if (!groups.success) {
    showSwapError(groups.error);
    return;
  }

  hideSwapError();
  setSwapLoading(true);

  const request = {
    action_groups: groups.items,
    value_balance: valueBalance,
    binding_sig: bindingSig,
  };

  try {
    const result = JSON.parse(wasm.build_swap_bundle(JSON.stringify(request)));
    if (result.success) {
      displaySwapBundleResult(result);
    } else {
      showSwapError(result.error || "Failed to build SwapBundle.");
    }
  } catch (error) {
    showSwapError(`SwapBundle error: ${error.message}`);
  } finally {
    setSwapLoading(false);
  }
}

function collectSwapGroups() {
  const groupsContainer = document.getElementById("swapGroups");
  if (!groupsContainer) {
    return { success: false, error: "Swap group container not found." };
  }

  const groupEls = Array.from(groupsContainer.querySelectorAll(".swap-group"));
  if (groupEls.length === 0) {
    return { success: false, error: "Add at least one action group." };
  }

  const groups = [];

  for (const groupEl of groupEls) {
    const flagsInput = groupEl.querySelector(".swap-group-flags");
    const anchorInput = groupEl.querySelector(".swap-group-anchor");
    const expiryInput = groupEl.querySelector(".swap-group-expiry");
    const proofInput = groupEl.querySelector(".swap-group-proof");

    const flags = flagsInput?.value.trim() || "";
    const anchor = anchorInput?.value.trim() || "";
    const expiryHeight = expiryInput?.value.trim() || "0";
    const proof = proofInput?.value.trim() || "";

    if (!flags) {
      return { success: false, error: "Flags are required for each action group." };
    }
    if (!anchor) {
      return { success: false, error: "Anchor is required for each action group." };
    }
    if (!proof) {
      return { success: false, error: "Proof is required for each action group." };
    }

    const actionRows = Array.from(groupEl.querySelectorAll(".swap-action-row"));
    if (actionRows.length === 0) {
      return { success: false, error: "Each action group must include at least one action." };
    }

    const actions = [];
    const spendAuthSigs = [];

    for (const row of actionRows) {
      const cvNet = row.querySelector(".swap-action-cv-net")?.value.trim() || "";
      const nullifier =
        row.querySelector(".swap-action-nullifier")?.value.trim() || "";
      const rk = row.querySelector(".swap-action-rk")?.value.trim() || "";
      const cmx = row.querySelector(".swap-action-cmx")?.value.trim() || "";
      const epk = row.querySelector(".swap-action-epk")?.value.trim() || "";
      const encCiphertext =
        row.querySelector(".swap-action-enc")?.value.trim() || "";
      const outCiphertext =
        row.querySelector(".swap-action-out")?.value.trim() || "";
      const spendAuthSig =
        row.querySelector(".swap-action-sig")?.value.trim() || "";

      if (!cvNet || !nullifier || !rk || !cmx || !epk) {
        return {
          success: false,
          error: "All action fields (cv_net, nullifier, rk, cmx, epk) are required.",
        };
      }
      if (!encCiphertext || !outCiphertext) {
        return {
          success: false,
          error: "enc_ciphertext and out_ciphertext are required for each action.",
        };
      }
      if (!spendAuthSig) {
        return {
          success: false,
          error: "Spend auth signature is required for each action.",
        };
      }

      actions.push({
        cv_net: cvNet,
        nullifier,
        rk,
        cmx,
        epk,
        enc_ciphertext: encCiphertext,
        out_ciphertext: outCiphertext,
      });
      spendAuthSigs.push(spendAuthSig);
    }

    const burnRows = Array.from(groupEl.querySelectorAll(".swap-burn-row"));
    const burns = [];

    for (const row of burnRows) {
      const asset = row.querySelector(".swap-burn-asset")?.value.trim() || "";
      const amount = row.querySelector(".swap-burn-amount")?.value.trim() || "";

      if (!asset || !amount) {
        return {
          success: false,
          error: "Burn entries require both asset and amount.",
        };
      }

      burns.push({
        asset,
        amount,
      });
    }

    groups.push({
      actions,
      flags,
      anchor,
      expiry_height: expiryHeight,
      burns,
      proof,
      spend_auth_sigs: spendAuthSigs,
    });
  }

  return { success: true, items: groups };
}

function displaySwapBundleResult(result) {
  const placeholder = document.getElementById("swapBundlePlaceholder");
  const output = document.getElementById("swapBundleResult");
  if (placeholder) placeholder.classList.add("d-none");
  if (output) output.classList.remove("d-none");

  const hexEl = document.getElementById("swapBundleHex");
  const valueBalanceEl = document.getElementById("swapBundleValueBalance");
  const bindingSigEl = document.getElementById("swapBundleBindingSig");
  const groupsEl = document.getElementById("swapBundleGroups");

  if (hexEl) hexEl.value = result.swap_bundle_hex || "";
  if (valueBalanceEl)
    valueBalanceEl.textContent =
      result.value_balance !== undefined ? String(result.value_balance) : "";
  if (bindingSigEl) bindingSigEl.textContent = result.binding_sig || "";
  if (groupsEl) groupsEl.value = JSON.stringify(result.action_groups || [], null, 2);
}

function setSwapLoading(isLoading) {
  const button = document.getElementById("swapBuildBtn");
  if (!button) return;

  button.disabled = isLoading;
  const spinner = button.querySelector(".loading-spinner");
  const btnText = button.querySelector(".btn-text");
  if (spinner) spinner.classList.toggle("d-none", !isLoading);
  if (btnText) btnText.classList.toggle("d-none", isLoading);
}

function showSwapError(message) {
  const errorEl = document.getElementById("swapBundleError");
  if (!errorEl) return;
  errorEl.textContent = message;
  errorEl.classList.remove("d-none");
}

function hideSwapError() {
  const errorEl = document.getElementById("swapBundleError");
  if (!errorEl) return;
  errorEl.classList.add("d-none");
  errorEl.textContent = "";
}

async function copySwapBundleHex() {
  const hexEl = document.getElementById("swapBundleHex");
  const button = document.getElementById("copySwapBundleHexBtn");
  if (!hexEl || !button) return;

  try {
    await navigator.clipboard.writeText(hexEl.value);
    const original = button.innerHTML;
    button.innerHTML = '<i class="bi bi-check2 me-1"></i> Copied';
    setTimeout(() => {
      button.innerHTML = original;
    }, 1500);
  } catch (error) {
    showSwapError("Failed to copy SwapBundle hex.");
  }
}
