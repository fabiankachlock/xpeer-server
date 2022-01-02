package xpeer

import (
	"log"

	"github.com/gofiber/fiber/v2"
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

type ServerConfig struct {
	Port           string
	Host           string
	VerboseLogging bool
}

type Server struct {
	app    *fiber.App
	Config ServerConfig
}

// globals
var (
	connectedPeers map[string]*Peer // stores all active connections
	app            *fiber.App       // fiber app
	logInfo        *log.Logger      // log infos
	logWarn        *log.Logger      // log warnings
	logError       *log.Logger      // log errors
)

// WriteMessage(messageType int, data []byte) error
// ReadMessage() (messageType int, p []byte, err error)

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
