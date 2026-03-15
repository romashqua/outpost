package core

import (
	commonv1 "github.com/romashqua/outpost/pkg/pb/outpost/common/v1"
	gatewayv1 "github.com/romashqua/outpost/pkg/pb/outpost/gateway/v1"
)

// hubPeerNotifier adapts StreamHub to the handler.PeerNotifier interface.
type hubPeerNotifier struct {
	hub *StreamHub
}

func (n *hubPeerNotifier) NotifyPeerAdd(pubkey string, allowedIPs []string) {
	n.hub.BroadcastPeerUpdate(&gatewayv1.PeerUpdate{
		Action: gatewayv1.PeerUpdate_ACTION_ADD,
		Peer: &commonv1.Peer{
			PublicKey:            pubkey,
			AllowedIps:          allowedIPs,
			PersistentKeepalive: 25,
		},
	})
}

func (n *hubPeerNotifier) NotifyPeerRemove(pubkey string) {
	n.hub.BroadcastPeerUpdate(&gatewayv1.PeerUpdate{
		Action: gatewayv1.PeerUpdate_ACTION_REMOVE,
		Peer: &commonv1.Peer{
			PublicKey: pubkey,
		},
	})
}
