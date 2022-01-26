package xpeer

// message handlers
var (
	handlerMap = map[string](func(msg WebsocketMessage)){
		OPR_SEND_DIRECT:        handleSendMessage,
		OPR_CREATE_V_PEER:      handleCreateVPeer,
		OPR_DELETE_V_PEER:      handleDeleteVPeer,
		OPR_CONNECT_V_PEER:     handleConnectVPeer,
		OPR_DISCONNECT_V_PEER:  handleDisconnectVPeer,
		OPR_PUT_SHARED_STATE:   handlePutState,
		OPR_PATCH_SHARED_STATE: handlePatchState,
		OPR_PING:               handlePing,
		OPR_PONG:               handlePong,
	}
)

// main handler for all incoming ws messages
func handleWebsocketMessage(raw string, sender string) {
	// parse raw message into struct
	msg, err := parseWebsocketMessage(raw, sender)
	if err != nil {
		logWarn.Printf("%s: errornous message %s\n", sender, err.Error())
		sendWebsocketMessage(MSG_ERROR, sender, sender, err.Error())
		return
	}
	logInfo.Printf("%s: receive %s::%s::{%d}\n", sender, msg.operation, msg.target, len(msg.payload))

	// get handler for message operation
	handler, ok := handlerMap[msg.operation]
	if !ok {
		logWarn.Printf("%s: errornous message %s\n", sender, ERR_UNKNOWN_OPERATION)
		sendWebsocketMessage(MSG_ERROR, sender, sender, ERR_UNKNOWN_OPERATION)
		return
	}

	// and execute operation
	handler(msg)
}

// handle send message operation
func handleSendMessage(msg WebsocketMessage) {
	err := sendWebsocketMessage(MSG_SEND, msg.sender, msg.target, msg.payload)
	if err == nil {
		sendSuccess(msg.sender, msg.target) // set target as payload (see xpeer spec)
	} else {
		if !sendError(msg.sender, err.Error()) {
			logError.Printf("%s: neither target (%s) nor sender are available\n", msg.sender, msg.target)
		}
	}
}

// handle ping operation
func handlePing(msg WebsocketMessage) {
	targetPeer := connectedPeers[msg.target]
	if targetPeer.isVirtual {
		sendWebsocketMessage(MSG_PONG, msg.target, msg.sender, "virtual")
		return
	}
	err := sendWebsocketMessage(MSG_PING, msg.sender, msg.target, msg.payload)
	if err != nil {
		sendError(msg.sender, err.Error())
	}
}

// handle pong operation (answer to ping operation)
func handlePong(msg WebsocketMessage) {
	// error can be discarded since the sender (target of ping) doesn't need to know if the answer has succeeded
	sendWebsocketMessage(MSG_PONG, msg.sender, msg.target, msg.payload)
}
