package xpeer

import (
	"log"
	"os"

	"github.com/fabiankachlock/xpeer-server/pkg/util"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/websocket/v2"
)

// establish websocket connections
func handleWebsocket(c *websocket.Conn) {
	// create new peer object
	peer := Peer{
		conn:      c,
		id:        util.GenerateId() + SERVER_PREFIX_DIVIDER + SERVER_SUFFIX,
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

func New() Server {
	app = fiber.New() // init fiber app
	config := getConfig()

	// initialize globals
	connectedPeers = map[string]*Peer{}
	logInfo = log.New(os.Stdout, "[XPeer] ", log.Ltime)
	logWarn = log.New(os.Stdout, "[XPeer] [Warn] ", log.Ltime)
	logError = log.New(os.Stdout, "[XPeer] [ERROR] ", log.Ltime)

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

	return Server{app: app, Config: config}
}

func (s Server) Start() {
	s.app.Listen(s.Config.Host + ":" + s.Config.Port)
}
