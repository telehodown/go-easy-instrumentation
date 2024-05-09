package main

import (
	"demo-app/pkg"
	"io"
	"log"
	"net/http"
)

func index(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "hello world")
}

func noticeError(w http.ResponseWriter, r *http.Request) {
	str, _, err := pkg.DoAThing(true)
	if err != nil {
		io.WriteString(w, err.Error())
	} else {
		io.WriteString(w, str+" no errors occured")
	}
}

func external(w http.ResponseWriter, r *http.Request) {
	req, err := http.NewRequest("GET", "https://google.com", nil)
	if err != nil {
		log.Fatal(err)
	}

	// Make an http request to an external address
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		io.WriteString(w, err.Error())
		return
	}

	defer resp.Body.Close()
	io.Copy(w, resp.Body)
}

func roundtripper(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{}

	request, err := http.NewRequest("GET", "https://example.com", nil)
	if err != nil {
		log.Fatal(err)
	}
	// Since the transaction is already added to the inbound request's
	// context by WrapHandleFunc, we just need to copy the context from the
	// inbound request to the external request.
	request = request.WithContext(r.Context())

	//	txn := newrelic.FromContext(r.Context())
	//	request = newrelic.RequestWithTransactionContext(request, txn)

	resp, err := client.Do(request)
	if err != nil {
		io.WriteString(w, err.Error())
		return
	}
	defer resp.Body.Close()
	io.Copy(w, resp.Body)
}
