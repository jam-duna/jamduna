package storage

import (
	"encoding/json"
	"fmt"

	"github.com/jam-duna/jamduna/common"
	log "github.com/jam-duna/jamduna/log"
	"github.com/jam-duna/jamduna/types"
)

// StoreBundleSpecSegments stores bundle and segment data for guarantors
func (s *StorageHub) StoreBundleSpecSegments(
	erasureRoot common.Hash,
	exportedSegmentRoot common.Hash,
	bChunks []types.DistributeECChunk,
	sChunks []types.DistributeECChunk,
	bClubs []common.Hash,
	sClubs []common.Hash,
	bundle []byte,
	encodedSegments []byte,
) error {
	// Store bundle chunks
	erasure_bKey := fmt.Sprintf("erasureBChunk-%v", erasureRoot)
	bChunkJson, _ := json.Marshal(bChunks)
	s.WriteRawKV([]byte(erasure_bKey), bChunkJson)

	// Store segment chunks (includes segments AND proof pages)
	erasure_sKey := fmt.Sprintf("erasureSChunk-%v", erasureRoot)
	sChunkJson, _ := json.Marshal(sChunks)
	s.WriteRawKV([]byte(erasure_sKey), sChunkJson)

	// Store bundle clubs
	erasure_bClubsKey := fmt.Sprintf("erasureBClubs-%v", erasureRoot)
	bClubsJson, _ := json.Marshal(bClubs)
	s.WriteRawKV([]byte(erasure_bClubsKey), bClubsJson)

	// Store segment clubs
	erasure_sClubsKey := fmt.Sprintf("erasureSClubs-%v", erasureRoot)
	sClubsJson, _ := json.Marshal(sClubs)
	s.WriteRawKV([]byte(erasure_sClubsKey), sClubsJson)

	// Store bundle for CE 148
	erasure_bundleKey := fmt.Sprintf("erasureBundle-%v", erasureRoot)
	if err := s.WriteRawKV([]byte(erasure_bundleKey), bundle); err != nil {
		log.Warn(log.Node, "StoreBundleSpecSegments: failed to persist bundle", "erasureRoot", erasureRoot, "err", err)
	}

	// Store segments for CE 147/148 GetSegmentWithProof lookups
	segmentsKey := fmt.Sprintf("erasureSegments-%v", exportedSegmentRoot)
	if err := s.WriteRawKV([]byte(segmentsKey), encodedSegments); err != nil {
		log.Warn(log.Node, "StoreBundleSpecSegments: failed to persist segments", "segmentsRoot", exportedSegmentRoot, "err", err)
	} else {
		log.Debug(log.DA, "StoreBundleSpecSegments: stored segments",
			"segmentsKey", segmentsKey,
			"exportedSegmentRoot", exportedSegmentRoot,
			"encodedLen", len(encodedSegments))
	}

	return nil
}

// GetGuarantorMetadata retrieves erasure metadata for guarantors
func (s *StorageHub) GetGuarantorMetadata(erasureRoot common.Hash) (
	bClubs []common.Hash,
	sClubs []common.Hash,
	bECChunks []types.DistributeECChunk,
	sECChunksArray []types.DistributeECChunk,
	err error,
) {
	erasure_bKey := fmt.Sprintf("erasureBChunk-%v", erasureRoot)
	erasure_bKey_val, ok, err := s.ReadRawKV([]byte(erasure_bKey))
	if err != nil || !ok {
		return
	}
	if err = json.Unmarshal(erasure_bKey_val, &bECChunks); err != nil {
		return
	}

	erasure_sKey := fmt.Sprintf("erasureSChunk-%v", erasureRoot)
	erasure_sKey_val, ok, err := s.ReadRawKV([]byte(erasure_sKey))
	if err != nil || !ok {
		return
	}
	if err = json.Unmarshal(erasure_sKey_val, &sECChunksArray); err != nil {
		return
	}

	erasure_bClubsKey := fmt.Sprintf("erasureBClubs-%v", erasureRoot)
	erasure_bClubs_val, ok, err := s.ReadRawKV([]byte(erasure_bClubsKey))
	if err != nil || !ok {
		return
	}
	if err = json.Unmarshal(erasure_bClubs_val, &bClubs); err != nil {
		return
	}

	erasure_sClubsKey := fmt.Sprintf("erasureSClubs-%v", erasureRoot)
	erasure_sClubs_val, ok, err := s.ReadRawKV([]byte(erasure_sClubsKey))
	if err != nil || !ok {
		return
	}
	if err = json.Unmarshal(erasure_sClubs_val, &sClubs); err != nil {
		return
	}
	return
}

// GetFullShard retrieves full shard data for guarantors (used in CE137 responses)
// Note: Justification generation happens in the node layer to avoid import cycles
func (s *StorageHub) GetFullShard(erasureRoot common.Hash, shardIndex uint16) (
	bundleShard []byte,
	segmentShards []byte,
	justification []byte,
	ok bool,
	err error,
) {
	// This is a placeholder - actual implementation with justification generation
	// must be done in the node layer to avoid import cycle with statedb/trie
	return nil, nil, nil, false, fmt.Errorf("GetFullShard must be called from node layer")
}

// StoreFullShardJustification stores shard justifications for assurers
func (s *StorageHub) StoreFullShardJustification(
	erasureRoot common.Hash,
	shardIndex uint16,
	bClub common.Hash,
	sClub common.Hash,
	encodedPath []byte,
) error {
	f_es_key := GenerateErasureRootShardIdxKey("f", erasureRoot, shardIndex)
	bundle_segment_pair := common.BuildBundleSegment(bClub, sClub)
	f_es_val := append(bundle_segment_pair, encodedPath...)
	s.WriteRawKV([]byte(f_es_key), f_es_val)
	return nil
}

// GetFullShardJustification retrieves shard justifications for assurers
func (s *StorageHub) GetFullShardJustification(erasureRoot common.Hash, shardIndex uint16) (
	bClubH common.Hash,
	sClubH common.Hash,
	encodedPath []byte,
	err error,
) {
	f_es_key := GenerateErasureRootShardIdxKey("f", erasureRoot, shardIndex)
	log.Trace(log.DA, "GetFullShardJustification", "f_es_key", f_es_key)

	data, ok, err := s.ReadRawKV([]byte(f_es_key))
	if err != nil || !ok {
		return
	}
	if len(data) < 64 {
		err = fmt.Errorf("GetFullShardJustification Bad data")
		return
	}
	bClubH = common.Hash(data[:32])
	sClubH = common.Hash(data[32:64])
	encodedPath = data[64:]
	return bClubH, sClubH, encodedPath, nil
}

// StoreAuditDA stores bundle shard for short-term audit (assurer role)
func (s *StorageHub) StoreAuditDA(erasureRoot common.Hash, shardIndex uint16, bundleShard []byte) error {
	b_es_key := GenerateErasureRootShardIdxKey("b", erasureRoot, shardIndex)
	s.WriteRawKV([]byte(b_es_key), bundleShard)
	log.Trace(log.DA, "StoreAuditDA", "b_es_key", b_es_key, "bundleShard", bundleShard)
	return nil
}

// StoreImportDA stores segment shards for long-term import (assurer role, at least 672 epochs)
func (s *StorageHub) StoreImportDA(erasureRoot common.Hash, shardIndex uint16, concatenatedShards []byte) error {
	s_es_key := GenerateErasureRootShardIdxKey("s", erasureRoot, shardIndex)
	s.WriteRawKV([]byte(s_es_key), concatenatedShards) // includes segment shards AND proof page shards
	return nil
}

// GetBundleShard retrieves bundle shard for assurers (used in CE138 responses)
func (s *StorageHub) GetBundleShard(erasureRoot common.Hash, shardIndex uint16) (
	bundleShard []byte,
	sClub common.Hash,
	justification []byte,
	ok bool,
	err error,
) {
	b_es_key := GenerateErasureRootShardIdxKey("b", erasureRoot, shardIndex)
	bundleShard, _, err = s.ReadRawKV([]byte(b_es_key))
	if err != nil {
		return nil, sClub, nil, false, err
	}

	_, sClub, encodedPath, err := s.GetFullShardJustification(erasureRoot, shardIndex)
	if err != nil {
		return nil, sClub, nil, false, err
	}

	return bundleShard, sClub, encodedPath, true, nil
}

// GetSegmentShard retrieves segment shards for assurers (used in CE139/CE140 responses)
func (s *StorageHub) GetSegmentShard(erasureRoot common.Hash, shardIndex uint16) (
	concatenatedShards []byte,
	ok bool,
	err error,
) {
	s_es_key := GenerateErasureRootShardIdxKey("s", erasureRoot, shardIndex)
	concatenatedShards, ok, err = s.ReadRawKV([]byte(s_es_key))
	if err != nil {
		return concatenatedShards, false, err
	}
	if !ok {
		return concatenatedShards, false, nil
	}
	return concatenatedShards, true, nil
}

// GetBundleByErasureRoot retrieves bundle by erasure root
func (s *StorageHub) GetBundleByErasureRoot(erasureRoot common.Hash) (types.WorkPackageBundle, bool) {
	erasure_bundleKey := fmt.Sprintf("erasureBundle-%v", erasureRoot)
	bundleBytes, ok, err := s.ReadRawKV([]byte(erasure_bundleKey))
	if err != nil || !ok {
		return types.WorkPackageBundle{}, false
	}

	bundle, _, err := types.DecodeBundle(bundleBytes)
	if err != nil {
		return types.WorkPackageBundle{}, false
	}
	return *bundle, true
}

// GetSegmentsBySegmentRoot retrieves segments by segment root
func (s *StorageHub) GetSegmentsBySegmentRoot(segmentRoot common.Hash) ([][]byte, bool) {
	segmentsKey := fmt.Sprintf("erasureSegments-%v", segmentRoot)
	encodedSegments, ok, err := s.ReadRawKV([]byte(segmentsKey))
	if err != nil || !ok {
		return nil, false
	}

	segmentsInterface, _, err := types.Decode(encodedSegments, nil)
	if err != nil {
		return nil, false
	}

	segments, ok := segmentsInterface.([][]byte)
	if !ok {
		return nil, false
	}
	return segments, true
}
