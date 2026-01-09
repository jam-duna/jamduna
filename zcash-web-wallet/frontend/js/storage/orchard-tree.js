// Zcash Web Wallet - Orchard Tree State Storage

import { STORAGE_KEYS } from "../constants.js";

// Load Orchard tree state from localStorage
export function loadOrchardTreeState() {
  const stored = localStorage.getItem(STORAGE_KEYS.orchardTreeState);
  if (stored) {
    try {
      return JSON.parse(stored);
    } catch {
      return null;
    }
  }
  return null;
}

// Save Orchard tree state to localStorage
export function saveOrchardTreeState(state) {
  localStorage.setItem(STORAGE_KEYS.orchardTreeState, JSON.stringify(state));
}

// Clear Orchard tree state
export function clearOrchardTreeState() {
  localStorage.removeItem(STORAGE_KEYS.orchardTreeState);
}
