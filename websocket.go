package xpeer

import (
	"errors"

	"github.com/gofiber/websocket/v2"
)

// general function for sending a message to a target
func sendWebsocketMessage(messageType string, sender string, target string, payload string) error {
	// build outgoing message
	receiverMsg := constructWebsocketMessage(messageType, sender, payload)

	// find target peer
	targetPeer, ok := connectedPeers[target]
	if ok {
		if targetPeer.isVirtual {
			// is virtual peer -> send message to all listening peers
			logInfo.Printf("%s: send broadcast from %s to %d peers", sender, target, len(targetPeer.broadcasts))
			broadcastMessage := constructWebsocketMessage(messageType, targetPeer.id, payload)
			// loop over all listeners
			for _, peerId := range targetPeer.broadcasts {
				// find listener connection
				if broadcastPeer, ok := connectedPeers[peerId]; ok {
					// send websocket message
					broadcastPeer.conn.WriteMessage(websocket.TextMessage, broadcastMessage)
				}
			}
		} else {
			// normal peer -> send websocket message
			logInfo.Printf("%s: send to %s", sender, target)
			targetPeer.conn.WriteMessage(websocket.TextMessage, receiverMsg)
		}
		return nil
	}
	return errors.New(ERR_TARGET_NOT_FOUND)
}

func sendSuccess(peerId string, payload string) bool {
	sendingError := sendWebsocketMessage(MSG_SUCCESS, peerId, peerId, payload)
	if sendingError != nil {
		// cannot send to initial sender -> discard operation quitely
		logWarn.Printf("initial sender not available: %s\n", peerId)
		return false
	}
	return true
}

func sendError(peerId string, err string, overrideSender ...string) bool {
	logWarn.Printf("%s: sending error %s", peerId, err)
	sender := peerId
	if len(overrideSender) > 0 {
		sender = overrideSender[0]
	}
	sendingError := sendWebsocketMessage(MSG_ERROR, sender, peerId, err)
	if sendingError != nil {
		// cannot send to initial sender -> discard operation quitely
		logError.Printf("initial sender not available: %s\n", peerId)
		return false
	}
	return true
}
