package main

import (
	"net/http"
)

// design pattern that forces awareness of call depth to pass instrumentation
func initHandlers() {
	http.HandleFunc("/", index)
	http.HandleFunc("/error", noticeError)
	http.HandleFunc("/external", external)
	http.HandleFunc("/roundtrip", roundtripper)
	http.HandleFunc("/basicExternal", basicExternal)
	http.HandleFunc("/async", async)
	http.HandleFunc("/async2", async2)
}

func main() {
	initHandlers()
	DoAThing(true)
	http.ListenAndServe(":8000", nil)
}
