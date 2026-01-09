// Zcash Web Wallet - Orchard Tree Sync

import {
  zGetBlockchainInfo,
  zGetTreeState,
  zGetSubtreesByIndex,
  zGetNotesCount,
} from "./rpc.js";
import {
  loadOrchardTreeState,
  saveOrchardTreeState,
} from "./storage/orchard-tree.js";
import { loadNotes, saveNotes } from "./storage/notes.js";

const DEFAULT_SUBTREE_LIMIT = 10;
const DEFAULT_POOL = "orchard";

function parseNumber(value) {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim() !== "") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return null;
}

function normalizeHex(value) {
  if (!value || typeof value !== "string") return null;
  return value.startsWith("0x") ? value.slice(2).toLowerCase() : value.toLowerCase();
}

function normalizeTreeState(raw, fallbackHeight = null) {
  if (!raw || typeof raw !== "object") return null;

  const orchard = raw.orchard || {};
  const commitments = orchard.commitments || {};

  const commitmentTreeSize = parseNumber(
    commitments.size ?? orchard.commitmentTreeSize
  );
  const commitmentTreeRoot =
    commitments.finalRoot ?? orchard.commitmentTreeRoot ?? null;
  const commitmentTreeState =
    commitments.finalState ?? orchard.commitmentTreeState ?? null;

  return {
    height: parseNumber(raw.height) ?? fallbackHeight,
    hash: raw.hash || null,
    time: parseNumber(raw.time),
    orchard: {
      commitmentTreeSize,
      commitmentTreeRoot,
      commitmentTreeState,
    },
  };
}

function normalizeNotesCount(raw) {
  if (raw == null) return null;
  if (typeof raw === "number") return raw;
  if (typeof raw === "string") return parseNumber(raw);
  if (typeof raw === "object") {
    return parseNumber(raw.orchard ?? raw.count ?? raw.total ?? null);
  }
  return null;
}

function normalizeSubtreesResponse(raw, fallbackStartIndex = 0, pool = DEFAULT_POOL) {
  if (!raw || typeof raw !== "object") {
    return { pool, start_index: fallbackStartIndex, subtrees: [] };
  }

  const responsePool = typeof raw.pool === "string" ? raw.pool : pool;
  const startIndex = parseNumber(raw.start_index) ?? fallbackStartIndex;
  const subtrees = Array.isArray(raw.subtrees) ? raw.subtrees : [];

  const normalized = subtrees.map((subtree, offset) => ({
    index: parseNumber(subtree.index) ?? startIndex + offset,
    root: subtree.root ?? null,
    height: parseNumber(subtree.height),
    end_height: parseNumber(subtree.end_height ?? subtree.endHeight),
    commitments: Array.isArray(subtree.commitments) ? subtree.commitments : null,
  }));

  return { pool: responsePool, start_index: startIndex, subtrees: normalized };
}

function mergeSubtrees(existing, incoming) {
  const merged = new Map();
  const passthrough = [];

  const existingList = Array.isArray(existing) ? existing : [];
  const incomingList = Array.isArray(incoming) ? incoming : [];

  for (const subtree of existingList) {
    if (Number.isInteger(subtree.index)) {
      merged.set(subtree.index, subtree);
    } else {
      passthrough.push(subtree);
    }
  }

  for (const subtree of incomingList) {
    if (Number.isInteger(subtree.index)) {
      const current = merged.get(subtree.index) || {};
      merged.set(subtree.index, { ...current, ...subtree });
    } else {
      passthrough.push(subtree);
    }
  }

  const ordered = Array.from(merged.entries())
    .sort((a, b) => a[0] - b[0])
    .map((entry) => entry[1]);

  return [...ordered, ...passthrough];
}

function updateOrchardNotePositions(notes, subtrees) {
  if (!Array.isArray(notes) || !Array.isArray(subtrees)) {
    return { notes, updated: false };
  }

  const positionByCommitment = new Map();

  for (const subtree of subtrees) {
    if (!Array.isArray(subtree.commitments) || subtree.commitments.length === 0) {
      continue;
    }

    const height = parseNumber(subtree.height);
    if (height == null || height < 0 || height > 31) {
      continue;
    }

    if (!Number.isInteger(subtree.index)) {
      continue;
    }

    const subtreeSize = Math.pow(2, height);
    const baseIndex = subtree.index * subtreeSize;

    for (let i = 0; i < subtree.commitments.length; i += 1) {
      const commitment = normalizeHex(subtree.commitments[i]);
      if (!commitment) continue;

      const position = baseIndex + i;
      if (!Number.isSafeInteger(position)) {
        continue;
      }
      positionByCommitment.set(commitment, position);
    }
  }

  if (positionByCommitment.size === 0) {
    return { notes, updated: false };
  }

  let updated = false;
  const updatedNotes = notes.map((note) => {
    if (note.pool !== "orchard" || !note.commitment) {
      return note;
    }

    const position = positionByCommitment.get(normalizeHex(note.commitment));
    if (position == null || note.orchard_position === position) {
      return note;
    }

    updated = true;
    return { ...note, orchard_position: position };
  });

  return { notes: updatedNotes, updated };
}

export async function syncOrchardTreeState(
  rpcEndpoint,
  {
    height = null,
    pool = DEFAULT_POOL,
    startIndex = null,
    limit = DEFAULT_SUBTREE_LIMIT,
    includeSubtrees = true,
    updateNotePositions = true,
  } = {}
) {
  if (!rpcEndpoint) {
    throw new Error("RPC endpoint is required for Orchard tree sync");
  }

  let storedState = loadOrchardTreeState();
  if (storedState?.endpoint && storedState.endpoint !== rpcEndpoint) {
    storedState = null;
  }

  let targetHeight = height;
  if (targetHeight == null) {
    try {
      const chainInfo = await zGetBlockchainInfo(rpcEndpoint);
      targetHeight = parseNumber(chainInfo?.blocks ?? chainInfo?.height);
    } catch (error) {
      console.warn("z_getblockchaininfo failed:", error);
    }
  }

  const treeStateRaw = await zGetTreeState(
    rpcEndpoint,
    targetHeight ?? "latest"
  );
  const treeState = normalizeTreeState(treeStateRaw, targetHeight);

  if (!treeState) {
    throw new Error("Failed to load Orchard tree state");
  }

  let notesCount = null;
  try {
    const notesCountRaw = await zGetNotesCount(rpcEndpoint);
    notesCount = normalizeNotesCount(notesCountRaw);
  } catch (error) {
    console.warn("z_getnotescount failed:", error);
  }

  let subtrees = Array.isArray(storedState?.subtrees) ? storedState.subtrees : [];
  if (includeSubtrees) {
    const startingIndex =
      startIndex ?? (Array.isArray(subtrees) ? subtrees.length : 0);

    try {
      const subtreesRaw = await zGetSubtreesByIndex(
        rpcEndpoint,
        pool,
        startingIndex,
        limit
      );
      const normalized = normalizeSubtreesResponse(
        subtreesRaw,
        startingIndex,
        pool
      );
      subtrees = mergeSubtrees(subtrees, normalized.subtrees);
    } catch (error) {
      console.warn("z_getsubtreesbyindex failed:", error);
    }
  }

  const state = {
    endpoint: rpcEndpoint,
    lastSyncedAt: new Date().toISOString(),
    treeState,
    notesCount,
    subtrees,
  };

  saveOrchardTreeState(state);

  let notesUpdated = false;
  if (updateNotePositions) {
    const notes = loadNotes();
    const updateResult = updateOrchardNotePositions(notes, subtrees);
    notesUpdated = updateResult.updated;
    if (notesUpdated) {
      saveNotes(updateResult.notes);
    }
  }

  return { state, notesUpdated };
}
