package main

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

func (s *Svc) Server(ctx context.Context) error {
	http.HandleFunc("/auth", s.addUser)
	http.HandleFunc("/update", s.forceUpdate)
	http.Handle("/metrics", promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{
		ErrorLog: log.Default(),
		Timeout:  2 * time.Second,
	}))

	addr := fmt.Sprintf("%s:%d", *s.options.host, *s.options.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err = http.Serve(listener, nil)
		wg.Done()
	}()
	log.Println("listening on ", listener.Addr())
	wg.Wait()
	return err
}

func (s *Svc) forceUpdate(w http.ResponseWriter, r *http.Request) {
	s.manualUpdate <- struct{}{}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Svc) addUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	err := r.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "could not parse form: %v\n", err)
		return
	}

	b := make([]byte, 10)
	_, err = rand.Read(b)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "could not generate key for OAuth exchange: %v\n", err)
		return
	}
	// TODO: Save state keys.

	state := base32.StdEncoding.EncodeToString(b)
	codeParam := r.Form["code"]
	if len(codeParam) == 0 {
		authCodeURL := s.oauthConfig.AuthCodeURL(state)
		http.Redirect(w, r, authCodeURL, http.StatusTemporaryRedirect)
		return
	}

	code := codeParam[0]
	tok, err := s.oauthConfig.Exchange(ctx, code)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "got error during exchange: %v\n", err)
		return
	}

	tokenSource := s.oauthConfig.TokenSource(ctx, tok)
	cal, err := s.Cal(ctx, tokenSource)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "could not connect to calendar service: %v\n", err)
		return
	}

	call := cal.Calendars.Get("primary")
	call.Context(ctx)
	entry, err := call.Do()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "could not get calendar ID: %v\n", err)
		return
	}
	err = s.StoreToken(ctx, entry.Id, tokenSource)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "failed to store token: %v\n", err)
		return
	}
}
