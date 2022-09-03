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

	flag.Parse()

	credentials, err := os.ReadFile(*credentialsFile)
	if err != nil {
		panic(err)
	}

	oauthConfig, err := google.ConfigFromJSON(credentials)
	if err != nil {
		log.Fatal("failed to get oauthconfig", err)
	}
	oauthConfig.Endpoint = google.Endpoint

	oauthConfig.Scopes = append(oauthConfig.Scopes, calendar.CalendarReadonlyScope)

	db, err := bolt.Open(*databaseFile, 0600, nil)
	if err != nil {
		panic(err)
	}

	svc := New(ctx, oauthConfig, db)

	go svc.BackgroundJob(ctx)

	err = svc.Server(ctx)
	if err != nil {
		panic(err)
	}
}
