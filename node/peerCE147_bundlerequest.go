package node

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

/*
CE 147: Bundle request
Request for a work-package bundle.

This protocol should be used by auditors to request work-package bundle from guarantors for auditing.
In case the guarantor fails to provide the valid bundle, the auditor should fall back to recovering
the bundle with CE 138.

Erasure-Root = [u8; 32]
Work-Package Bundle = As in GP

Auditor -> Guarantor

--> Erasure-Root
--> FIN
<-- Work-Package Bundle
<-- FIN
*/

// SendBundleRequest sends a bundle request to a guarantor (auditor side)
func (p *Peer) SendBundleRequest(ctx context.Context, erasureRoot common.Hash, eventID uint64) (*types.WorkPackageBundle, error) {
	code := uint8(CE147_BundleRequest)

	// Telemetry: Sending bundle request (event 148)
	p.node.telemetryClient.SendingBundleRequest(eventID, p.PeerKey())

	stream, err := p.openStream(ctx, code)
	if err != nil {
		// Telemetry: Bundle request failed (event 150)
		p.node.telemetryClient.BundleRequestFailed(eventID, err.Error())
		return nil, fmt.Errorf("openStream[CE147_BundleRequest]: %w", err)
	}

	// --> Erasure-Root
	erasureRootBytes := erasureRoot.Bytes()
	if err := sendQuicBytes(ctx, stream, erasureRootBytes, p.SanKey(), code); err != nil {
		// Telemetry: Bundle request failed (event 150)
		p.node.telemetryClient.BundleRequestFailed(eventID, err.Error())
		return nil, fmt.Errorf("sendQuicBytes[CE147_BundleRequest]: %w", err)
	}

	// Telemetry: Bundle request sent (event 151)
	p.node.telemetryClient.BundleRequestSent(eventID)

	// --> FIN
	stream.Close()

	// <-- Work-Package Bundle
	bundleBytes, err := receiveQuicBytes(ctx, stream, p.SanKey(), code)
	if err != nil {
		// Telemetry: Bundle request failed (event 150)
		p.node.telemetryClient.BundleRequestFailed(eventID, err.Error())
		return nil, fmt.Errorf("receiveQuicBytes[CE147_BundleRequest]: %w", err)
	}

	// Decode the bundle
	bundle, _, err := types.DecodeBundle(bundleBytes)
	if err != nil {
		// Telemetry: Bundle request failed (event 150)
		p.node.telemetryClient.BundleRequestFailed(eventID, err.Error())
		return nil, fmt.Errorf("DecodeBundle[CE147_BundleRequest]: %w", err)
	}

	// Telemetry: Bundle transferred (event 153)
	p.node.telemetryClient.BundleTransferred(eventID)

	log.Trace(log.Node, "CE147-SendBundleRequest",
		"node", p.node.id,
		"peerKey", p.SanKey(),
		"erasureRoot", erasureRoot,
		"bundleSize", len(bundleBytes),
		"workPackageHash", bundle.WorkPackage.Hash(),
	)

	return bundle, nil
}

// onBundleRequest handles incoming bundle requests (guarantor side)
func (n *Node) onBundleRequest(ctx context.Context, stream quic.Stream, msg []byte, peerKey string) error {
	defer stream.Close()

	// Get peer to access its PeerID for telemetry
	peer, ok := n.peersByPubKey[peerKey]
	if !ok {
		return fmt.Errorf("onBundleRequest: peer not found for key %s", peerKey)
	}

	// Telemetry: Receiving bundle request (event 149)
	eventID := n.telemetryClient.GetEventID()
	n.telemetryClient.ReceivingBundleRequest(PubkeyBytes(peer.Validator.Ed25519.SAN()))

	// Parse erasure root
	if len(msg) != 32 {
		// Telemetry: Bundle request failed (event 150)
		n.telemetryClient.BundleRequestFailed(eventID, "invalid erasure root length")
		return fmt.Errorf("onBundleRequest: invalid erasure root length: expected 32, got %d", len(msg))
	}

	erasureRoot := common.BytesToHash(msg)

	// Telemetry: Bundle request received (event 152)
	n.telemetryClient.BundleRequestReceived(eventID, erasureRoot)

	log.Trace(log.Node, "CE147-onBundleRequest INCOMING",
		"node", n.id,
		"peerKey", peerKey,
		"erasureRoot", erasureRoot,
	)

	// Look up the bundle
	bundle, ok := n.getBundleByErasureRoot(erasureRoot)
	if !ok {
		log.Warn(log.Node, "onBundleRequest: bundle not found",
			"node", n.id,
			"erasureRoot", erasureRoot,
		)
		// Telemetry: Bundle request failed (event 150)
		n.telemetryClient.BundleRequestFailed(eventID, "bundle not found")
		// Cancel the stream to indicate not found (consistent with other CE protocols)
		stream.CancelWrite(ErrKeyNotFound)
		return fmt.Errorf("onBundleRequest: bundle not found for erasureRoot=%v", erasureRoot)
	}

	// <-- Work-Package Bundle
	bundleBytes := bundle.Bytes()

	code := uint8(CE147_BundleRequest)
	if err := sendQuicBytes(ctx, stream, bundleBytes, n.GetEd25519Key().SAN(), code); err != nil {
		// Telemetry: Bundle request failed (event 150)
		n.telemetryClient.BundleRequestFailed(eventID, err.Error())
		return fmt.Errorf("onBundleRequest: sendQuicBytes failed: %w", err)
	}

	// Telemetry: Bundle transferred (event 153)
	n.telemetryClient.BundleTransferred(eventID)

	log.Trace(log.Node, "CE147-onBundleRequest SENT",
		"node", n.id,
		"peerKey", peerKey,
		"erasureRoot", erasureRoot,
		"bundleSize", len(bundleBytes),
		"workPackageHash", bundle.WorkPackage.Hash(),
	)

	return nil
}

// getBundleByErasureRoot retrieves a work package bundle from storage by its erasure root
func (n *Node) getBundleByErasureRoot(erasureRoot common.Hash) (types.WorkPackageBundle, bool) {
	// Create the storage key using the same pattern as StoreBundleSpec
	erasure_bundleKey := fmt.Sprintf("erasureBundle-%v", erasureRoot)

	// Read the bundle bytes from storage
	bundleBytes, ok, err := n.store.ReadRawKV([]byte(erasure_bundleKey))
	if err != nil {
		log.Warn(log.Node, "getBundleByErasureRoot: ReadRawKV failed",
			"erasureRoot", erasureRoot,
			"err", err)
		return types.WorkPackageBundle{}, false
	}

	if !ok {
		log.Trace(log.Node, "getBundleByErasureRoot: bundle not found",
			"erasureRoot", erasureRoot)
		return types.WorkPackageBundle{}, false
	}

	// Decode the bundle
	bundle, _, err := types.DecodeBundle(bundleBytes)
	if err != nil {
		log.Warn(log.Node, "getBundleByErasureRoot: DecodeBundle failed",
			"erasureRoot", erasureRoot,
			"err", err)
		return types.WorkPackageBundle{}, false
	}

	log.Trace(log.Node, "getBundleByErasureRoot: bundle found",
		"erasureRoot", erasureRoot,
		"bundleSize", len(bundleBytes),
		"workPackageHash", bundle.WorkPackage.Hash())

	return *bundle, true
}
