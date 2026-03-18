package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/yampug/btw/internal/remote"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("btw-agent version %s\n", version)
		return
	}

	logger := log.New(os.Stderr, "btw-agent: ", log.LstdFlags|log.Lmsgprefix)
	logger.Println("starting")

	dec := remote.NewDecoder(os.Stdin)
	enc := remote.NewEncoder(os.Stdout)

	server := remote.NewAgentServer(logger)
	server.Handle(remote.MethodWalk, remote.HandleWalk)
	server.Handle(remote.MethodGrep, remote.HandleGrep)
	server.Handle(remote.MethodSymbols, remote.HandleSymbols)
	server.Handle(remote.MethodDetectRoot, remote.HandleDetectRoot)
	server.Handle(remote.MethodReadIgnore, remote.HandleReadIgnore)

	if err := server.Serve(context.Background(), dec, enc); err != nil {
		logger.Printf("fatal: %v", err)
		os.Exit(1)
	}

	logger.Println("shutdown complete")
}
