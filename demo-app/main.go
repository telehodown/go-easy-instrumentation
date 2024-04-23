package main

import (
	"errors"
	"io"
	"net/http"
)

func index(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "hello world")
}

func noticeError(w http.ResponseWriter, r *http.Request) {
	err := errors.New("an error has occured")
	if err != nil {
		io.WriteString(w, err.Error())
	} else {
		io.WriteString(w, "no errors occured")
	}
}

func main() {
	http.HandleFunc("/", index)
	http.HandleFunc("/error", noticeError)

	http.ListenAndServe(":8000", nil)
}
