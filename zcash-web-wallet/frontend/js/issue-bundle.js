// Zcash Web Wallet - IssueBundle Builder (NU7)

import { getWasm } from "./wasm.js";

let issueNoteCounter = 0;

export function initIssueBundleUI() {
  const form = document.getElementById("issueBundleForm");
  const addNoteBtn = document.getElementById("issueAddNoteBtn");
  const generateKeyBtn = document.getElementById("issueGenerateKeyBtn");
  const generateAssetBtn = document.getElementById("issueGenerateAssetHashBtn");
  const copyHexBtn = document.getElementById("copyIssueBundleHexBtn");

  if (!form) return;

  if (addNoteBtn) {
    addNoteBtn.addEventListener("click", () => addIssueNoteRow());
  }

  if (generateKeyBtn) {
    generateKeyBtn.addEventListener("click", generateIssuerKey);
  }

  if (generateAssetBtn) {
    generateAssetBtn.addEventListener("click", generateAssetDescHash);
  }

  if (copyHexBtn) {
    copyHexBtn.addEventListener("click", copyIssueBundleHex);
  }

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    buildIssueBundle();
  });

  addIssueNoteRow();
}

function addIssueNoteRow() {
  const notesContainer = document.getElementById("issueNotes");
  if (!notesContainer) return;

  const row = document.createElement("div");
  row.className = "border rounded p-2 mb-2 issue-note-row";
  row.dataset.noteId = String(issueNoteCounter++);

  row.innerHTML = `
    <div class="row g-2 align-items-end">
      <div class="col-12">
        <label class="form-label">Recipient</label>
        <input type="text" class="form-control mono issue-note-recipient" placeholder="Unified address or raw Orchard hex" />
      </div>
      <div class="col-md-4">
        <label class="form-label">Value (u64)</label>
        <input type="text" class="form-control issue-note-value" placeholder="100000000" />
      </div>
      <div class="col-md-4">
        <label class="form-label">Rho (optional)</label>
        <input type="text" class="form-control mono issue-note-rho" placeholder="32-byte hex" />
      </div>
      <div class="col-md-4">
        <label class="form-label">Rseed (optional)</label>
        <input type="text" class="form-control mono issue-note-rseed" placeholder="32-byte hex" />
      </div>
      <div class="col-12 text-end">
        <button type="button" class="btn btn-sm btn-outline-danger issue-note-remove">
          Remove
        </button>
      </div>
    </div>
  `;

  const removeBtn = row.querySelector(".issue-note-remove");
  if (removeBtn) {
    removeBtn.addEventListener("click", () => {
      row.remove();
    });
  }

  notesContainer.appendChild(row);
}

function generateAssetDescHash() {
  const input = document.getElementById("issueAssetDescHash");
  if (!input) return;

  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  input.value = bytesToHex(bytes);
}

function generateIssuerKey() {
  const wasm = getWasm();
  if (!wasm) {
    showIssueError("WASM module not loaded.");
    return;
  }

  try {
    const result = JSON.parse(wasm.generate_issue_auth_key());
    if (result.success) {
      const secretInput = document.getElementById("issueIssuerSecret");
      const publicInput = document.getElementById("issueIssuerKey");
      if (secretInput) secretInput.value = result.issuer_secret_key || "";
      if (publicInput) publicInput.value = result.issuer_key || "";
    } else {
      showIssueError(result.error || "Failed to generate issuer key.");
    }
  } catch (error) {
    showIssueError(`Key generation error: ${error.message}`);
  }
}

function buildIssueBundle() {
  const wasm = getWasm();
  if (!wasm) {
    showIssueError("WASM module not loaded.");
    return;
  }

  const issuerSecretInput = document.getElementById("issueIssuerSecret");
  const issuerKeyInput = document.getElementById("issueIssuerKey");
  const assetDescInput = document.getElementById("issueAssetDescHash");
  const finalizeInput = document.getElementById("issueFinalize");

  const issuerSecret = issuerSecretInput?.value.trim() || "";
  const assetDescHash = assetDescInput?.value.trim() || "";
  const finalize = Boolean(finalizeInput?.checked);

  const notes = collectNotes();
  if (!notes.success) {
    showIssueError(notes.error);
    return;
  }

  if (!assetDescHash) {
    showIssueError("Asset description hash is required.");
    return;
  }

  hideIssueError();
  setIssueLoading(true);

  const request = {
    issuer_secret_key: issuerSecret || null,
    actions: [
      {
        asset_desc_hash: assetDescHash,
        finalize,
        notes: notes.items,
      },
    ],
  };

  try {
    const result = JSON.parse(wasm.build_issue_bundle(JSON.stringify(request)));
    if (result.success) {
      displayIssueBundleResult(result);
      if (issuerKeyInput && result.issuer_key) {
        issuerKeyInput.value = result.issuer_key;
      }
      if (issuerSecretInput && result.issuer_secret_key) {
        issuerSecretInput.value = result.issuer_secret_key;
      }
    } else {
      showIssueError(result.error || "Failed to build IssueBundle.");
    }
  } catch (error) {
    showIssueError(`IssueBundle error: ${error.message}`);
  } finally {
    setIssueLoading(false);
  }
}

function collectNotes() {
  const notesContainer = document.getElementById("issueNotes");
  if (!notesContainer) {
    return { success: false, error: "Notes container not found." };
  }

  const rows = Array.from(notesContainer.querySelectorAll(".issue-note-row"));
  if (rows.length === 0) {
    return { success: false, error: "Add at least one note." };
  }

  const notes = [];
  for (const row of rows) {
    const recipientInput = row.querySelector(".issue-note-recipient");
    const valueInput = row.querySelector(".issue-note-value");
    const rhoInput = row.querySelector(".issue-note-rho");
    const rseedInput = row.querySelector(".issue-note-rseed");

    const recipient = recipientInput?.value.trim() || "";
    const value = valueInput?.value.trim() || "";
    const rho = rhoInput?.value.trim() || "";
    const rseed = rseedInput?.value.trim() || "";

    if (!recipient) {
      return { success: false, error: "Recipient is required for each note." };
    }
    if (!value) {
      return { success: false, error: "Value is required for each note." };
    }

    notes.push({
      recipient,
      value,
      rho: rho || null,
      rseed: rseed || null,
    });
  }

  return { success: true, items: notes };
}

function displayIssueBundleResult(result) {
  const placeholder = document.getElementById("issueBundlePlaceholder");
  const output = document.getElementById("issueBundleResult");
  if (placeholder) placeholder.classList.add("d-none");
  if (output) output.classList.remove("d-none");

  const hexEl = document.getElementById("issueBundleHex");
  const issuerKeyEl = document.getElementById("issueBundleIssuerKey");
  const sigEl = document.getElementById("issueBundleSignature");
  const commitmentEl = document.getElementById("issueBundleCommitment");
  const actionsEl = document.getElementById("issueBundleActions");

  if (hexEl) hexEl.value = result.issue_bundle_hex || "";
  if (issuerKeyEl) issuerKeyEl.textContent = result.issuer_key || "";
  if (sigEl) sigEl.textContent = result.signature || "";
  if (commitmentEl) commitmentEl.textContent = result.commitment || "";
  if (actionsEl) {
    actionsEl.value = JSON.stringify(result.actions || [], null, 2);
  }
}

function setIssueLoading(isLoading) {
  const button = document.getElementById("issueBuildBtn");
  if (!button) return;

  button.disabled = isLoading;
  const spinner = button.querySelector(".loading-spinner");
  const btnText = button.querySelector(".btn-text");
  if (spinner) spinner.classList.toggle("d-none", !isLoading);
  if (btnText) btnText.classList.toggle("d-none", isLoading);
}

function showIssueError(message) {
  const errorEl = document.getElementById("issueBundleError");
  if (!errorEl) return;
  errorEl.textContent = message;
  errorEl.classList.remove("d-none");
}

function hideIssueError() {
  const errorEl = document.getElementById("issueBundleError");
  if (!errorEl) return;
  errorEl.classList.add("d-none");
  errorEl.textContent = "";
}

async function copyIssueBundleHex() {
  const hexEl = document.getElementById("issueBundleHex");
  const button = document.getElementById("copyIssueBundleHexBtn");
  if (!hexEl || !button) return;

  try {
    await navigator.clipboard.writeText(hexEl.value);
    const original = button.innerHTML;
    button.innerHTML = '<i class="bi bi-check2 me-1"></i> Copied';
    setTimeout(() => {
      button.innerHTML = original;
    }, 1500);
  } catch (error) {
    showIssueError("Failed to copy IssueBundle hex.");
  }
}

function bytesToHex(bytes) {
  return Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}
