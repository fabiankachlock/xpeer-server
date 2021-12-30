package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/websocket/v2"
)

// wrapper for all websocket connection (virtual or not)
type Peer struct {
	conn       *websocket.Conn // reference to a active websocket connection (not used with vpeers)
	id         string          // unique connection identifier
	isVirtual  bool            // virtual or not indicator
	broadcasts []string        // only vpeer: all connection id that listens to this peer
	listens    []string        // only peer: all connection id that that peers listens to
	state      string          // only vpeer: shared state
}

// wrapper for parsed incoming websocket message
type WebsocketMessage struct {
	operation string
	sender    string
	target    string
	payload   string
}

var (
	connectedPeers map[string]*Peer // stores all active connections
	app            *fiber.App       // fiber app
	logInfo        *log.Logger      // log infos
	logWarn        *log.Logger      // log warnings
	logError       *log.Logger      // log errors
)

// all constants for ids
const (
	SERVER_PREFIX_LENGTH  = 5
	PEER_ID_LENGTH        = 16
	SERVER_SUFFIX         = "_dev_"
	SERVER_PREFIX_DIVIDER = "@"
)

// all constants for operations
const (
	OPR_LENGTH             = 8
	OPR_ID_LENGTH          = SERVER_PREFIX_LENGTH + 1 + PEER_ID_LENGTH
	OPR_DIVIDER_LENGTH     = 2
	OPR_DIVIDER            = "::"
	OPR_PING               = "sendPing"
	OPR_PONG               = "sendPong"
	OPR_CREATE_V_PEER      = "crtVPeer"
	OPR_DELETE_V_PEER      = "delVPeer"
	OPR_CONNECT_V_PEER     = "conVPeer"
	OPR_DISCONNECT_V_PEER  = "disVPeer"
	OPR_SEND_DIRECT        = "sendPeer"
	OPR_PUT_SHARED_STATE   = "putState"
	OPR_PATCH_SHARED_STATE = "patState"
)

// all constants for outgoing messages
const (
	MSG_TYPE_LENGTH  = 8
	MSG_DIVIDER      = "::"
	MSG_SUCCESS      = "oprResOk"
	MSG_SEND         = "recvPeer"
	MSG_PING         = "sendPing"
	MSG_PONG         = "sendPong"
	MSG_PEER_ID      = "gPeerCId"
	MSG_ERROR        = "errorMsg"
	MSG_STATE_UPDATE = "stateMut"
)

// all errors
const (
	ERR_MESSAGE_TOO_SHORT          = "error: message too short"
	ERR_INVALID_MESSAGE            = "error: invalid message format"
	ERR_UNKNOWN_OPERATION          = "error: message operation is unknown"
	ERR_TARGET_NOT_FOUND           = "error: target could not be located"
	ERR_STATE_MUTATION_NOT_ALLOWED = "error: state mutation of peers is not allowed"
)

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
			// loop over all listeners
			for _, peerId := range targetPeer.broadcasts {
				// find listener connection
				if broadcastPeer, ok := connectedPeers[peerId]; ok {
					// send websocket message
					broadcastPeer.conn.WriteMessage(websocket.TextMessage, receiverMsg)
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

// handle send message operation
func handleSendMessage(msg WebsocketMessage) {
	err := sendWebsocketMessage(MSG_SEND, msg.sender, msg.target, msg.payload)
	if err == nil {
		// error ca be discarded since if the sender is not available, no one can be notified
		sendWebsocketMessage(MSG_SUCCESS, msg.sender, msg.sender, msg.target) // set target as payload (see xpeer spec)
	} else {
		anewErr := sendWebsocketMessage(MSG_ERROR, msg.sender, msg.sender, err.Error())
		logWarn.Printf("%s: unknown target %s\n", msg.sender, msg.target)
		if anewErr != nil {
			// cannot send to sender -> discard operation quitely
			logError.Printf("%s: neither target (%s) nor sender are available\n", msg.sender, msg.target)
		}
	}
}

// handle ping operation
func handlePing(msg WebsocketMessage) {
	err := sendWebsocketMessage(MSG_PING, msg.sender, msg.target, msg.payload)
	if err != nil {
		// error ca be discarded since if the sender is not available, no one can be notified
		sendWebsocketMessage(MSG_ERROR, msg.sender, msg.sender, err.Error())
		logWarn.Printf("%s: unknown target %s\n", msg.sender, msg.target)
	}
}

// handle pong operation (answer to ping operation)
func handlePong(msg WebsocketMessage) {
	// error can be discarded since the sender (target of ping) doesn't need to know if the answer has succeeded
	sendWebsocketMessage(MSG_PONG, msg.sender, msg.target, msg.payload)
}

// create a new virtual peer
func handleCreateVPeer(msg WebsocketMessage) {
	// construct new peer object
	peer := Peer{
		id:         generateId(),
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
		// error ca be discarded since if the sender is not available, no one can be notified
		sendWebsocketMessage(MSG_ERROR, msg.sender, msg.sender, err.Error())
		logWarn.Printf("%s: unknown target %s\n", msg.sender, msg.target)
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
				peer.listens = filterSliceByPeerId(peer.listens, vpeer.id)
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

	// TODO: check if vpeer & send error or success
	if vpeerOk && peerOk {
		logInfo.Printf("connect %s to vpeer %s\n", msg.sender, msg.target)
		vpeer.broadcasts = append(vpeer.broadcasts, msg.sender)
		peer.listens = append(peer.listens, msg.target)
		sendWebsocketMessage(MSG_STATE_UPDATE, vpeer.id, peer.id, msg.payload)
	}
}

// disconnect a peer from a vpeer
func handleDisconnectVPeer(msg WebsocketMessage) {
	logInfo.Printf("disconnect %s from vpeer %s\n", msg.sender, msg.target)

	// TODO: check if vpeer & send error or success
	if vpeer, ok := connectedPeers[msg.target]; ok {
		vpeer.broadcasts = filterSliceByPeerId(vpeer.broadcasts, msg.sender)
	}

	if peer, ok := connectedPeers[msg.target]; ok {
		peer.listens = filterSliceByPeerId(peer.listens, msg.target)
	}
}

// TODO: Fix error handling
// override the state ov a vpeer
func handlePutState(msg WebsocketMessage) {
	// find vpeer
	vpeer, ok := connectedPeers[msg.target]
	if ok {
		if !vpeer.isVirtual {
			sendWebsocketMessage(MSG_ERROR, vpeer.id, msg.target, ERR_STATE_MUTATION_NOT_ALLOWED)
			return
		}
		// override state
		vpeer.state = msg.payload
		logInfo.Printf("%s: put state of %s", msg.sender, msg.target)
		sendWebsocketMessage(MSG_STATE_UPDATE, vpeer.id, msg.target, msg.payload)
		return
	}
	sendWebsocketMessage(MSG_ERROR, msg.sender, msg.sender, ERR_TARGET_NOT_FOUND)
}

// TODO: Fix error handling
// update state of a vpeer
func handlePatchState(msg WebsocketMessage) {
	vpeer, ok := connectedPeers[msg.target]
	if ok {
		if !vpeer.isVirtual {
			sendWebsocketMessage(MSG_ERROR, vpeer.id, msg.target, ERR_STATE_MUTATION_NOT_ALLOWED)
			return
		}
		// merge state
		before := map[string]interface{}{}
		after := map[string]interface{}{}
		json.Unmarshal([]byte(vpeer.state), &before)
		json.Unmarshal([]byte(msg.payload), &after)
		newState := mergeState(before, after)
		// set new state
		jsonEncoded, err := json.Marshal(newState)
		if err == nil {
			vpeer.state = string(jsonEncoded)
			logInfo.Printf("%s: patch state of %s", msg.sender, msg.target)
		}

		sendWebsocketMessage(MSG_STATE_UPDATE, vpeer.id, msg.target, msg.payload)
		return
	}
	sendWebsocketMessage(MSG_ERROR, msg.sender, msg.sender, ERR_TARGET_NOT_FOUND)
}

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

// establish websocket connections
func handleWebsocket(c *websocket.Conn) {
	// create new peer object
	peer := Peer{
		conn:      c,
		id:        generateId(),
		isVirtual: false,      // cant be a virtual peer since its coming from a real connection
		listens:   []string{}, // init without listening to any other peer
	}

	// store ref to connection & send id to peer
	connectedPeers[peer.id] = &peer
	peer.conn.WriteMessage(websocket.TextMessage, constructWebsocketMessage(MSG_PEER_ID, peer.id, peer.id))
	logInfo.Printf("connected %s\n", peer.id)

	// allocate msg & err only once instead of in every loop
	var (
		msg []byte
		err error
	)

	// listen to incoming messages
	for {
		if _, msg, err = c.ReadMessage(); err != nil {
			logError.Printf("%s: %s\n", peer.id, err.Error())
			break
		}
		handleWebsocketMessage(string(msg), peer.id)
	}

	// remove from active connections
	delete(connectedPeers, peer.id)
	logInfo.Printf("disconnected %s\n", peer.id)
}

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

// initialize and start fiber app
func startServer() {
	app = fiber.New() // init fiber app

	// apply middelware
	app.Use(logger.New(logger.Config{
		Format: "[Fiber] ${time} ${method} ${path} - ${status} (${latency})\n",
	}))
	app.Use(recover.New())

	// TODO: dev only
	// expose an endpoint for an overview over all connected peer ids
	app.Get("/peers", func(c *fiber.Ctx) error {
		peers := ""
		for key, val := range connectedPeers {
			peers += key
			if val.isVirtual {
				peers += " (virtual)"
			}
			peers += "\n"
		}
		c.SendString(peers)
		return nil
	})

	// expose ping endpoint for availability testing
	app.Get("/ping", func(c *fiber.Ctx) error {
		return c.SendString("pong")
	})

	// expose main xpeer websocket endpoint
	app.Get("/xpeer", websocket.New(handleWebsocket))

	// start app
	app.Listen("127.0.0.1:3000")
}

// server entry point
func main() {
	// initialize globals
	connectedPeers = map[string]*Peer{}
	logInfo = log.New(os.Stdout, "[XPeer] ", log.Ltime)
	logWarn = log.New(os.Stdout, "[XPeer] [Warn] ", log.Ltime)
	logError = log.New(os.Stdout, "[XPeer] [ERROR] ", log.Ltime)

	startServer()
}

// util

// return given slice without specified string (id)
func filterSliceByPeerId(slice []string, id string) []string {
	newSlice := make([]string, len(slice)-1)
	for _, elm := range slice {
		if elm != id {
			newSlice = append(newSlice, elm)
		}
	}
	return newSlice
}

const (
	RAW_ID_LENGTH = 12 // results in a 16 char base64 encoded string
)

// generate a secure unique random id in valid xpeer-spec format
func generateId() string {
	b := make([]byte, RAW_ID_LENGTH)

	_, err := rand.Read(b) // generate secure random bytes
	if err != nil {
		logError.Println(err.Error())
		return "<<<err-id-gen>>>" // return error indicating placeholder id
	}

	// return base64 encoded random bytes appended with server id
	return base64.RawURLEncoding.EncodeToString(b) + SERVER_PREFIX_DIVIDER + SERVER_SUFFIX
}
