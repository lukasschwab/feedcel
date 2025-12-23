package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/lukasschwab/feedcel/pkg/proxy"
)

func main() {
	port := flag.Int("port", 8080, "Port to listen on")
	flag.Parse()

	filterer, err := proxy.NewFilterer(nil)
	if err != nil {
		log.Fatalf("Failed to initialize filterer: %v", err)
	}

	http.HandleFunc("/filter", filterer.Handle)
	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Starting proxy server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}
