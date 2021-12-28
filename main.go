package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/websocket/v2"
)

type Peer struct {
	conn       *websocket.Conn
	id         string
	isVirtual  bool
	broadcasts []string
	listens    []string
	state      string
}

type WebsocketMessage struct {
	operation string
	sender    string
	target    string
	payload   string
}

var (
	connectedPeers map[string]*Peer
)

const (
	SERVER_PREFIX_LENGTH  = 5
	PEER_ID_LENGTH        = 16
	SERVER_PREFIX         = "_dev_"
	SERVER_PREFIX_DIVIDER = "@"
)

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

const (
	MSG_TYPE_LENGTH  = 8
	MSG_DIVIDER      = "::"
	MSG_SEND         = "recvPeer"
	MSG_PING         = "sendPing"
	MSG_PONG         = "sendPong"
	MSG_PEER_ID      = "gPeerCId"
	MSG_ERROR        = "errorMsg"
	MSG_STATE_UPDATE = "stateMut"
)

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

func sendWebsocketMessage(messageType string, sender string, target string, payload string) {
	receiverMsg := constructWebsocketMessage(messageType, sender, payload)
	targetPeer, ok := connectedPeers[target]
	if ok {
		if targetPeer.isVirtual {
			fmt.Println("Broadcast to", targetPeer.broadcasts)
			for _, peerId := range targetPeer.broadcasts {
				if broadcastPeer, ok := connectedPeers[peerId]; ok {
					broadcastPeer.conn.WriteMessage(websocket.TextMessage, receiverMsg)
				}
			}
		} else {
			targetPeer.conn.WriteMessage(websocket.TextMessage, receiverMsg)
		}
		return
	}

	senderPeer, ok := connectedPeers[sender]
	if ok {
		senderPeer.conn.WriteMessage(websocket.TextMessage, constructWebsocketMessage(MSG_ERROR, sender, ERR_TARGET_NOT_FOUND))
		return
	}
	fmt.Println("[ERROR]: Neither Target nor Sender are available")
}

func handleSendMessage(msg WebsocketMessage) {
	sendWebsocketMessage(MSG_SEND, msg.sender, msg.target, msg.payload)
}

func handlePing(msg WebsocketMessage) {
	sendWebsocketMessage(MSG_PING, msg.sender, msg.target, msg.payload)
}

func handlePong(msg WebsocketMessage) {
	sendWebsocketMessage(MSG_PONG, msg.sender, msg.target, msg.payload)
}

func handleCreateVPeer(msg WebsocketMessage) {
	peer := Peer{
		id:         generateId(),
		isVirtual:  true,
		broadcasts: []string{msg.sender},
		state:      "",
	}
	connectedPeers[peer.id] = &peer
	fmt.Printf("Create VPeer %s\n", peer.id)

	senderPeer, ok := connectedPeers[msg.sender]
	if ok {
		senderPeer.listens = append(senderPeer.listens, peer.id)
		senderPeer.conn.WriteMessage(websocket.TextMessage, constructWebsocketMessage(MSG_PEER_ID, peer.id, peer.id))
		return
	}
}

func handleDeleteVPeer(msg WebsocketMessage) {
	fmt.Printf("Delete VPeer %s\n", msg.target)
	if vpeer, ok := connectedPeers[msg.target]; ok {
		for _, peerId := range vpeer.broadcasts {
			if peer, ok := connectedPeers[peerId]; ok {
				peer.listens = filterSliceByPeerId(peer.listens, vpeer.id)
			}
		}
	}
}

func handleConnectVPeer(msg WebsocketMessage) {
	vpeer, vpeerOk := connectedPeers[msg.target]
	peer, peerOk := connectedPeers[msg.sender]

	if vpeerOk && peerOk {
		fmt.Printf("Connect %s to VPeer %s\n", msg.sender, msg.target)
		vpeer.broadcasts = append(vpeer.broadcasts, msg.sender)
		peer.listens = append(peer.listens, msg.target)
		sendWebsocketMessage(MSG_STATE_UPDATE, vpeer.id, peer.id, msg.payload)
	}
}

func handleDisconnectVPeer(msg WebsocketMessage) {
	fmt.Printf("Disconnect %s from VPeer %s\n", msg.sender, msg.target)

	if vpeer, ok := connectedPeers[msg.target]; ok {
		vpeer.broadcasts = filterSliceByPeerId(vpeer.broadcasts, msg.sender)
	}

	if peer, ok := connectedPeers[msg.target]; ok {
		peer.listens = filterSliceByPeerId(peer.listens, msg.target)
	}
}

func handlePutState(msg WebsocketMessage) {
	vpeer, ok := connectedPeers[msg.target]
	if ok {
		if !vpeer.isVirtual {
			sendWebsocketMessage(MSG_ERROR, vpeer.id, msg.target, ERR_STATE_MUTATION_NOT_ALLOWED)
			return
		}
		vpeer.state = msg.payload
		sendWebsocketMessage(MSG_STATE_UPDATE, vpeer.id, msg.target, msg.payload)
		return
	}
	sendWebsocketMessage(MSG_ERROR, msg.sender, msg.sender, ERR_TARGET_NOT_FOUND)
}

func handlePatchState(msg WebsocketMessage) {
	vpeer, ok := connectedPeers[msg.target]
	if ok {
		if !vpeer.isVirtual {
			sendWebsocketMessage(MSG_ERROR, vpeer.id, msg.target, ERR_STATE_MUTATION_NOT_ALLOWED)
			return
		}
		before := map[string]interface{}{}
		after := map[string]interface{}{}
		json.Unmarshal([]byte(vpeer.state), &before)
		json.Unmarshal([]byte(msg.payload), &after)

		newState := mergeState(before, after)
		jsonEncoded, err := json.Marshal(newState)
		if err == nil {
			vpeer.state = string(jsonEncoded)
		}

		sendWebsocketMessage(MSG_STATE_UPDATE, vpeer.id, msg.target, msg.payload)
		return
	}
	sendWebsocketMessage(MSG_ERROR, msg.sender, msg.sender, ERR_TARGET_NOT_FOUND)
}

func mergeState(before map[string]interface{}, after map[string]interface{}) map[string]interface{} {
	newState := before

	for key, val := range after {
		existing, ok := newState[key]
		if ok {
			switch existing.(type) {
			case map[string]interface{}:
				newState[key] = mergeState(existing.(map[string]interface{}), after[key].(map[string]interface{}))
				break
			default:
				newState[key] = val
			}
		} else {
			newState[key] = val
		}
	}

	return newState
}

// message parsing

const (
	OPR_START         = 0
	OPR_END           = OPR_LENGTH
	OPR_TARGET_START  = OPR_END + OPR_DIVIDER_LENGTH
	OPR_TARGET_END    = OPR_LENGTH + OPR_DIVIDER_LENGTH + OPR_ID_LENGTH
	OPR_PAYLOAD_START = OPR_TARGET_END + OPR_DIVIDER_LENGTH
)

func parseWebsocketMessage(msg string, sender string) (WebsocketMessage, error) {
	if !(len(msg) >= OPR_PAYLOAD_START) {
		return WebsocketMessage{}, errors.New(ERR_MESSAGE_TOO_SHORT)
	}

	var (
		opr     = msg[OPR_START:OPR_END]
		target  = msg[OPR_TARGET_START:OPR_TARGET_END]
		payload = msg[OPR_PAYLOAD_START:]
	)

	if msg[OPR_END:OPR_END+OPR_DIVIDER_LENGTH] != OPR_DIVIDER || msg[OPR_TARGET_END:OPR_TARGET_END+OPR_DIVIDER_LENGTH] != OPR_DIVIDER {
		return WebsocketMessage{}, errors.New(ERR_INVALID_MESSAGE)
	}

	return WebsocketMessage{
		operation: opr,
		sender:    sender,
		target:    target,
		payload:   payload,
	}, nil
}

// message building
func constructWebsocketMessage(msgType string, sender string, payload string) []byte {
	return []byte(msgType + MSG_DIVIDER + sender + MSG_DIVIDER + payload)
}

// ws server
func handleWebsocket(c *websocket.Conn) {
	// websocket.Conn bindings https://pkg.go.dev/github.com/fasthttp/websocket?tab=doc#pkg-index
	peer := Peer{
		conn:      c,
		id:        generateId(),
		isVirtual: false,
		listens:   []string{},
	}
	connectedPeers[peer.id] = &peer

	peer.conn.WriteMessage(websocket.TextMessage, constructWebsocketMessage(MSG_PEER_ID, peer.id, peer.id))

	var (
		msg []byte
		err error
	)

	for {
		if _, msg, err = c.ReadMessage(); err != nil {
			if websocket.IsCloseError(err) {
				delete(connectedPeers, peer.id)
			}
			fmt.Printf("err: %s\n", err)
			break
		}
		fmt.Printf("recv: %s\n", msg)
		handleWebsocketMessage(string(msg), peer.id)
	}
}

func handleWebsocketMessage(raw string, sender string) {
	msg, err := parseWebsocketMessage(raw, sender)
	if err != nil {
		errorMsg := WebsocketMessage{
			target:    sender,
			sender:    sender,
			operation: OPR_SEND_DIRECT,
			payload:   err.Error(),
		}
		handleSendMessage(errorMsg)
		return
	}

	handler, ok := handlerMap[msg.operation]
	if !ok {
		errorMsg := WebsocketMessage{
			target:    sender,
			sender:    sender,
			operation: OPR_SEND_DIRECT,
			payload:   ERR_UNKNOWN_OPERATION,
		}
		handleSendMessage(errorMsg)
		return
	}

	handler(msg)
}

func startServer() {
	app := fiber.New()
	app.Use(logger.New())
	app.Use(recover.New())

	app.Get("/ping", func(c *fiber.Ctx) error {
		return c.SendString("pong")
	})

	app.Get("/xpeer", websocket.New(handleWebsocket))
	app.Listen("127.0.0.1:3000")
}

func main() {
	connectedPeers = map[string]*Peer{}
	startServer()
}

// util

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
	RAW_ID_LENGTH = 12
)

func generateId() string {
	b := make([]byte, RAW_ID_LENGTH)
	_, err := rand.Read(b)
	if err != nil {
		fmt.Println("error:", err)
		return "<<<err-id-gen>>>"
	}
	return base64.RawURLEncoding.EncodeToString(b) + SERVER_PREFIX_DIVIDER + SERVER_PREFIX
}
