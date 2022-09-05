package main

import (
	"context"
	"flag"
	"github.com/boltdb/bolt"
	"google.golang.org/api/calendar/v3"
	"log"
	"os"

	"golang.org/x/oauth2/google"
)

func main() {
	ctx := context.Background()

	credentialsFile := flag.String("goog.credentials", "./data/google.json", "JSON key for Google credentials")
	databaseFile := flag.String("bolt.db", "./data/db.bolt", "Database file location (created if not exists)")

	opts := options{
		host: flag.String("net.host", "127.0.0.1", "Hostname for listening to HTTP requests. Defaults to 127.0.0.1."),
		port: flag.Int("net.port", 9994, "Port for listening to HTTP request. Defaults to 9994."),
	}

	flag.Parse()

	credentials, err := os.ReadFile(*credentialsFile)
	if err != nil {
		log.Fatalf("could not read credentials: %v\n", err)
	}

	oauthConfig, err := google.ConfigFromJSON(credentials)
	if err != nil {
		log.Fatalf("failed to get OAuth config: %v\n", err)
	}
	oauthConfig.Endpoint = google.Endpoint

	oauthConfig.Scopes = append(oauthConfig.Scopes, calendar.CalendarReadonlyScope)

	db, err := bolt.Open(*databaseFile, 0600, nil)
	if err != nil {
		log.Fatalf("failed to read database file: %v\n", err)
	}

	svc, err := New(ctx, opts, oauthConfig, db)
	if err != nil {
		log.Fatalf("failed to create service: %v\n")
	}

	go svc.BackgroundJob(ctx)

	err = svc.Server(ctx)
	if err != nil {
		panic(err)
	}
}
