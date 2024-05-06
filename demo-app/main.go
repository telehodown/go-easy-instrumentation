package main

import (
	"net/http"
)

func main() {
	http.HandleFunc("/", index)
	http.HandleFunc("/error", noticeError)

	http.ListenAndServe(":8000", nil)
}
