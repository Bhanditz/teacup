package main

import (
	"fmt"
	"log"
	"net"
	"os"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

const defaultPort = 8686

var (
	app = kingpin.New("teacup", "A cozy debugging JSON-over-RPC TCP proxy")
)

func main() {
	_, err := app.Parse(os.Args[1:])
	if err != nil {
		ctx, _ := app.ParseContext(os.Args[1:])
		if ctx != nil {
			app.FatalUsageContext(ctx, "%s\n", err.Error())
		} else {
			app.FatalUsage("%s\n", err.Error())
		}
	}

	start()
}

func start() {
	address := fmt.Sprintf("localhost:%d", defaultPort)
	listener, err := net.Listen("tcp", address)
	must(err)
	log.Printf("Teacup proxy listening on %s", address)

	for {
		acceptOne(listener)
	}
}

func acceptOne(listener net.Listener) {
	conn, err := listener.Accept()
	if err != nil {
		log.Printf("While accepting: %+v", err)
		return
	}

	go handleConn(conn)
}

func must(err error) {
	if err != nil {
		panic(fmt.Sprintf("fatal error: %+v", err))
	}
}
