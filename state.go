package xpeer

import "encoding/json"

// merge two json object recursively
func mergeState(before map[string]interface{}, after map[string]interface{}) map[string]interface{} {
	newState := before // declare a new state with the value of the old state

	// loop over all keys in the json object
	for key, val := range after {
		existing, ok := newState[key]
		if ok {
			// it already exists inn the current state -> check type of value
			// type JSON-Object (map[string]interface{}): merge recursively
			// other type (string,number,bool): override current value with new value
			switch existing.(type) {
			case map[string]interface{}:
				newState[key] = mergeState(existing.(map[string]interface{}), after[key].(map[string]interface{})) // recursive call
			default: // exit condition
				newState[key] = val
			}
		} else {
			// it doesn't already exist in the current state -> add it to the new state
			newState[key] = val
		}
	}

	// return merged state
	return newState
}

// override the state ov a vpeer
func handlePutState(msg WebsocketMessage) {
	// find vpeer
	vpeer, ok := connectedPeers[msg.target]
	if ok {
		if !vpeer.isVirtual {
			sendError(msg.sender, ERR_PEER_NOT_VIRTUAL, vpeer.id)
			return
		}
		// override state
		vpeer.state = msg.payload
		logInfo.Printf("%s: put state of %s", msg.sender, msg.target)
		sendSuccess(msg.sender, msg.target) // set target as payload (see xpeer spec)
		sendWebsocketMessage(MSG_STATE_UPDATE, vpeer.id, msg.target, msg.payload)
		return
	}
	sendError(msg.sender, ERR_TARGET_NOT_FOUND)
}

// update state of a vpeer
func handlePatchState(msg WebsocketMessage) {
	vpeer, ok := connectedPeers[msg.target]
	if ok {
		if !vpeer.isVirtual {
			sendError(msg.sender, ERR_PEER_NOT_VIRTUAL, vpeer.id)
			return
		}
		// merge state
		before := map[string]interface{}{}
		json.Unmarshal([]byte(vpeer.state), &before)
		after := map[string]interface{}{}
		json.Unmarshal([]byte(msg.payload), &after)
		newState := mergeState(before, after)

		// encode new state
		jsonEncoded, err := json.Marshal(newState)
		if err != nil {
			sendError(msg.sender, ERR_INVALID_STATE_FORMAT, vpeer.id)
			return
		}

		// set new state
		vpeer.state = string(jsonEncoded)
		logInfo.Printf("%s: patch state of %s", msg.sender, msg.target)

		sendSuccess(msg.sender, msg.target) // set target as payload (see xpeer spec)
		sendWebsocketMessage(MSG_STATE_UPDATE, vpeer.id, msg.target, msg.payload)
		return
	}
	sendError(msg.sender, ERR_TARGET_NOT_FOUND)
}
