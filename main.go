package main

import (
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/websocket/v2"
	"github.com/google/uuid"
)

type Peer struct {
	conn       *websocket.Conn
	id         string
	isVirtual  bool
	broadcasts []string
	listens    []string
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
	OPR_LENGTH             = 8
	OPR_ID_LENGTH          = 36
	OPR_DIVIDER_LENGTH     = 2
	OPR_DIVIDER            = "::"
	OPR_CREATE_V_PEER      = "crtVPeer"
	OPR_DELETE_V_PEER      = "delVPeer"
	OPR_CONNECT_V_PEER     = "conVPeer"
	OPR_DISCONNECT_V_PEER  = "disVPeer"
	OPR_SEND_DIRECT        = "sendPeer"
	OPR_PUT_SHARED_STATE   = "putState"
	OPR_PATCH_SHARED_STATE = "patState"
)

const (
	MSG_TYPE_LENGTH = 8
	MSG_DIVIDER     = "::"
	MSG_SEND        = "recvPeer"
	MSG_PEER_ID     = "gPeerCId"
	MSG_ERROR       = "errorMsg"
)

const (
	ERR_MESSAGE_TOO_SHORT = "error: message too short"
	ERR_INVALID_MESSAGE   = "error: invalid message format"
	ERR_UNKNOWN_OPERATION = "error: message operation is unknown"
	ERR_TARGET_NOT_FOUND  = "error: target could not be located"
)

// message handlers
var (
	handlerMap = map[string](func(msg WebsocketMessage)){
		OPR_SEND_DIRECT:       handleSendMessage,
		OPR_CREATE_V_PEER:     handleCreateVPeer,
		OPR_DELETE_V_PEER:     handleDeleteVPeer,
		OPR_CONNECT_V_PEER:    handleConnectVPeer,
		OPR_DISCONNECT_V_PEER: handleDisconnectVPeer,
	}
)

func handleSendMessage(msg WebsocketMessage) {
	receiverMsg := constructWebsocketMessage(MSG_SEND, msg.sender, msg.payload)
	targetPeer, ok := connectedPeers[msg.target]
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

	senderPeer, ok := connectedPeers[msg.sender]
	if ok {
		senderPeer.conn.WriteMessage(websocket.TextMessage, constructWebsocketMessage(MSG_ERROR, msg.sender, ERR_TARGET_NOT_FOUND))
		return
	}
	fmt.Println("[ERROR]: Neither Target nor Sender are available")
}

func handleCreateVPeer(msg WebsocketMessage) {
	peer := Peer{
		id:         uuid.NewString(),
		isVirtual:  true,
		broadcasts: []string{msg.sender},
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
		id:        uuid.NewString(),
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
		newSlice = append(newSlice, elm)
	}
	return newSlice
}
