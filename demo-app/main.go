package main

import (
	"net/http"
)

func main() {
	// Handle Functions
	http.HandleFunc("/", index)
	http.HandleFunc("/error", noticeError)
	http.HandleFunc("/external", external)
	http.HandleFunc("/roundtrip", roundtripper)

	http.ListenAndServe(":8000", nil)
}
