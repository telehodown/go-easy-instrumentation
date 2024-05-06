package main

import (
	"demo-app/pkg"
	"io"
	"net/http"
)

func index(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "hello world")
}

func noticeError(w http.ResponseWriter, r *http.Request) {
	err := pkg.DoAThing(true)
	if err != nil {
		io.WriteString(w, err.Error())
	} else {
		io.WriteString(w, "no errors occured")
	}
}
