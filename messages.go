package xpeer

import "errors"

// all constants for message parsing
const (
	OPR_START         = 0
	OPR_END           = OPR_LENGTH
	OPR_TARGET_START  = OPR_END + OPR_DIVIDER_LENGTH
	OPR_TARGET_END    = OPR_LENGTH + OPR_DIVIDER_LENGTH + OPR_ID_LENGTH
	OPR_PAYLOAD_START = OPR_TARGET_END + OPR_DIVIDER_LENGTH
)

// parse a xpeer spec conform incoming websocket message
func parseWebsocketMessage(msg string, sender string) (WebsocketMessage, error) {
	// message can't be shorter than one with payload length of zero
	if !(len(msg) >= OPR_PAYLOAD_START+0) {
		return WebsocketMessage{}, errors.New(ERR_MESSAGE_TOO_SHORT)
	}

	// extract information from message
	var (
		opr     = msg[OPR_START:OPR_END]
		target  = msg[OPR_TARGET_START:OPR_TARGET_END]
		payload = msg[OPR_PAYLOAD_START:]
	)

	// validate diver positions
	if msg[OPR_END:OPR_END+OPR_DIVIDER_LENGTH] != OPR_DIVIDER || msg[OPR_TARGET_END:OPR_TARGET_END+OPR_DIVIDER_LENGTH] != OPR_DIVIDER {
		return WebsocketMessage{}, errors.New(ERR_INVALID_MESSAGE)
	}

	// return final parsed message
	return WebsocketMessage{
		operation: opr,
		sender:    sender,
		target:    target,
		payload:   payload,
	}, nil
}

// build outgoing websocket message conform to the xpeer spec
func constructWebsocketMessage(msgType string, sender string, payload string) []byte {
	return []byte(msgType + MSG_DIVIDER + sender + MSG_DIVIDER + payload)
}
