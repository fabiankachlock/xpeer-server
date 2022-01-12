package main

import (
	"log"

	"github.com/fabiankachlock/xpeer-server"
)

func main() {
	log.Fatalln(xpeer.New().Start())
}
