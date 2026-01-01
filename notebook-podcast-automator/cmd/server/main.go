package main

import (
	"log"
	"notebook-podcast-automator/internal/httpserver"
)

func main() {
	if err := httpserver.Run(); err != nil {
		log.Fatal(err)
	}
}
