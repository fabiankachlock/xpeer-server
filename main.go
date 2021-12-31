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
	"github.com/spf13/viper"
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

type serverConfig struct {
	port string
	host string
}

// globals
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
	ERR_MESSAGE_TOO_SHORT    = "error: message too short"
	ERR_INVALID_MESSAGE      = "error: invalid message format"
	ERR_UNKNOWN_OPERATION    = "error: message operation is unknown"
	ERR_TARGET_NOT_FOUND     = "error: target could not be located"
	ERR_PEER_NOT_VIRTUAL     = "error: that target peer is not virtual"
	ERR_INVALID_STATE_FORMAT = "error: the state string is formatted invalidly"
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
		peer.listens = filterSliceByPeerId(peer.listens, msg.target)
	} else {
		// server answer can be discarded, since the receiver of success/error isn't available
		return
	}

	if vpeer, ok := connectedPeers[msg.target]; ok {
		if vpeer.isVirtual {
			// remove from the vpeers broadcasts
			vpeer.broadcasts = filterSliceByPeerId(vpeer.broadcasts, msg.sender)
			sendSuccess(msg.sender, vpeer.id)
		} else {
			sendError(msg.sender, ERR_PEER_NOT_VIRTUAL)
		}
	}
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

		sendWebsocketMessage(MSG_STATE_UPDATE, vpeer.id, msg.target, msg.payload)
		return
	}
	sendError(msg.sender, ERR_TARGET_NOT_FOUND)
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
func startServer(config serverConfig) {
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
	app.Listen(config.host + ":" + config.port)
}

// server entry point
func main() {
	// initialize globals
	connectedPeers = map[string]*Peer{}
	logInfo = log.New(os.Stdout, "[XPeer] ", log.Ltime)
	logWarn = log.New(os.Stdout, "[XPeer] [Warn] ", log.Ltime)
	logError = log.New(os.Stdout, "[XPeer] [ERROR] ", log.Ltime)

	config := getConfig()
	startServer(config)
}

// util

// read config from xpeer.env
func getConfig() serverConfig {
	// configure viper to read xpeer.env
	viper.SetConfigName("xpeer")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()
	viper.SetConfigType("env")

	// set default values
	viper.SetDefault("XPEER_HOST", "0.0.0.0")
	viper.SetDefault("XPEER_PORT", "8192")

	// read config file
	if err := viper.ReadInConfig(); err != nil {
		logWarn.Printf("error reading config: %s", err)
	}

	// read configured values
	host, hostOk := viper.Get("XPEER_HOST").(string)
	port, portOk := viper.Get("XPEER_PORT").(string)
	if !hostOk || !portOk {
		logError.Fatalf("Invalid type assertion")
	}

	// return config
	return serverConfig{
		host: host,
		port: port,
	}
}

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
