package main

import (
	"flag"
	"fmt"
	"os"

	"clawmonitor/internal/handler"
)

const VERSION = "v2.5"

func main() {
	port := flag.Int("port", 8899, "API server port")
	flag.Parse()

	fmt.Printf("ClawMonitor %s\n", VERSION)
	fmt.Printf("Starting API server on port %d...\n\n", *port)

	server := handler.NewServer(*port)
	if err := server.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
