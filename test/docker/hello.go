package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	http.HandleFunc("/hello", func(writer http.ResponseWriter, request *http.Request) {
		fmt.Fprintf(writer, "Hello, %s", os.Getenv("INSTANCE_ID"))
	})
	log.Fatal(http.ListenAndServe(":8080", nil)) //nolint:gosec
}
