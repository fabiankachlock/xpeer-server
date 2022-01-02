package xpeer

import "github.com/fabiankachlock/xpeer-server/pkg/util"

// create a new virtual peer
func handleCreateVPeer(msg WebsocketMessage) {
	// construct new peer object
	peer := Peer{
		id:         util.GenerateId() + SERVER_PREFIX_DIVIDER + SERVER_SUFFIX,
		isVirtual:  true,       // make peer virtual
		broadcasts: []string{}, // init empty listeners array
		state:      "{}",       // init empty state
	}

	// store new peer
	connectedPeers[peer.id] = &peer
	logInfo.Printf("%s: create vpeer %s\n", msg.sender, peer.id)

	// notify peer specified as target with the new vpeers id
	err := sendWebsocketMessage(MSG_PEER_ID, msg.target, msg.target, peer.id)
	if err != nil {
		sendError(msg.sender, err.Error())
	}
}

// delete a virtual peer
func handleDeleteVPeer(msg WebsocketMessage) {
	logInfo.Printf("delete vpeer %s\n", msg.target)
	// find targeted peer and check if its virtual
	if vpeer, ok := connectedPeers[msg.target]; ok && vpeer.isVirtual {
		// remove from listens array of all broadcast targets
		for _, peerId := range vpeer.broadcasts {
			if peer, ok := connectedPeers[peerId]; ok {
				peer.listens = util.FilterSliceByPeerId(peer.listens, vpeer.id)
			}
		}
		// delete from connected peers
		delete(connectedPeers, vpeer.id)
	}
}

// connect a peer to a vpeer
func handleConnectVPeer(msg WebsocketMessage) {
	vpeer, vpeerOk := connectedPeers[msg.target]
	peer, peerOk := connectedPeers[msg.sender]

	// booth peer have to be available for a connection
	if vpeerOk && peerOk {
		// check if the target peer is actual a vpeer
		if vpeer.isVirtual {
			logInfo.Printf("connect %s to vpeer %s\n", msg.sender, msg.target)
			// store the new connection
			vpeer.broadcasts = append(vpeer.broadcasts, msg.sender)
			peer.listens = append(peer.listens, msg.target)
			// initial state update for connected peer
			// TODO: make sure they actually arrive in the right order
			sendSuccess(msg.sender, vpeer.id)
			sendWebsocketMessage(MSG_STATE_UPDATE, vpeer.id, peer.id, msg.payload)
			return
		} else {
			sendError(msg.sender, ERR_PEER_NOT_VIRTUAL)
		}
	} else {
		sendError(msg.sender, ERR_TARGET_NOT_FOUND)
	}
}

// disconnect a peer from a vpeer
func handleDisconnectVPeer(msg WebsocketMessage) {
	logInfo.Printf("disconnect %s from vpeer %s\n", msg.sender, msg.target)

	if peer, ok := connectedPeers[msg.target]; ok {
		// remove from the peers listenings
		peer.listens = util.FilterSliceByPeerId(peer.listens, msg.target)
	} else {
		// server answer can be discarded, since the receiver of success/error isn't available
		return
	}

	if vpeer, ok := connectedPeers[msg.target]; ok {
		if vpeer.isVirtual {
			// remove from the vpeers broadcasts
			vpeer.broadcasts = util.FilterSliceByPeerId(vpeer.broadcasts, msg.sender)
			sendSuccess(msg.sender, vpeer.id)
		} else {
			sendError(msg.sender, ERR_PEER_NOT_VIRTUAL)
		}
	}
}
