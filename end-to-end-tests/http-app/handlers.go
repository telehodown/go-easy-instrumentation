package main

import (
	"errors"
	"http-app/pkg"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// the most basic http handler function
func index(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "hello world")
}

func DoAThing(willError bool) (string, bool, error) {
	time.Sleep(200 * time.Millisecond)
	if willError {
		return "thing not done", false, errors.New("this is an error")
	}

	return "thing complete", true, nil
}

func noticeError(w http.ResponseWriter, r *http.Request) {
	err := pkg.Service()
	if err != nil {
		io.WriteString(w, err.Error())
		return
	}

	str, _, err := DoAThing(true)
	if err != nil {
		io.WriteString(w, err.Error())
	} else {
		io.WriteString(w, str+" no errors occured")
	}
}

func external(w http.ResponseWriter, r *http.Request) {
	req, err := http.NewRequest("GET", "https://example.com", nil)
	if err != nil {
		slog.Error(err.Error())
		return
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

func basicExternal(w http.ResponseWriter, r *http.Request) {
	// Make an http request to an external address
	resp, err := http.Get("https://example.com")
	if err != nil {
		slog.Error(err.Error())
		io.WriteString(w, err.Error())
		return
	}

	defer resp.Body.Close()
	io.Copy(w, resp.Body)
}

func roundtripper(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{}
	client2 := client // verify that this doesn't get the transport replaced by the parser

	request, err := http.NewRequest("GET", "https://example.com", nil)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	resp, err := client2.Do(request)

	// this is an unusual spacing and comment pattern to test the decoration preservation
	if err != nil {
		slog.Error(err.Error())
		io.WriteString(w, err.Error())
		return
	}

	defer resp.Body.Close()
	io.Copy(w, resp.Body)
}

func async(w http.ResponseWriter, r *http.Request) {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
	}()
	wg.Wait()
	w.Write([]byte("done!"))
}

func doAsyncThing(wg *sync.WaitGroup) {
	defer wg.Done()
	time.Sleep(100 * time.Millisecond)
	_, err := http.Get("http://example.com")
	if err != nil {
		slog.Error(err.Error())
	}
}

func async2(w http.ResponseWriter, r *http.Request) {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go doAsyncThing(wg)
	wg.Wait()
	w.Write([]byte("done!"))
}
