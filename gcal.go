package main

import (
	"context"
	"encoding/json"
	"github.com/boltdb/bolt"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
	"log"
	"sort"
	"time"
)

type Svc struct {
	oauthConfig *oauth2.Config
	database    *bolt.DB
	options     options

	manualUpdate chan struct{}

	gaugeNextMeetingDuration *prometheus.GaugeVec
	gaugeNextMeetingEpoch    *prometheus.GaugeVec

	registry *prometheus.Registry
}

type options struct {
	host *string
	port *int
}

func New(ctx context.Context, opts options, oauthConfig *oauth2.Config, db *bolt.DB) (*Svc, error) {
	gaugeNextMeetingDuration := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: "calendar",
		Name:      "next_meeting_seconds",
	}, []string{"calendar", "type"})
	gaugeNextMeetingEpoch := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: "calendar",
		Name:      "next_meeting_epoch_seconds",
	}, []string{"calendar", "type"})

	reg := prometheus.NewRegistry()
	err := reg.Register(gaugeNextMeetingDuration)
	if err != nil {
		return nil, err
	}
	err = reg.Register(gaugeNextMeetingEpoch)
	if err != nil {
		return nil, err
	}

	return &Svc{
		options:      opts,
		oauthConfig:  oauthConfig,
		database:     db,
		manualUpdate: make(chan struct{}, 100),

		gaugeNextMeetingEpoch:    gaugeNextMeetingEpoch,
		gaugeNextMeetingDuration: gaugeNextMeetingDuration,

		registry: reg,
	}, nil
}

func (s *Svc) Cal(ctx context.Context, tokenSource oauth2.TokenSource) (*calendar.Service, error) {
	return calendar.NewService(ctx, option.WithTokenSource(tokenSource))
}

var bucketTokens = []byte("tokens")

func (s *Svc) StoreToken(ctx context.Context, key string, tokenSource oauth2.TokenSource) error {
	err := s.database.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(bucketTokens)
		if err != nil {
			return err
		}

		tok, err := tokenSource.Token()
		if err != nil {
			return err
		}

		b, err := json.Marshal(tok)
		if err != nil {
			return err
		}

		return bucket.Put([]byte(key), b)
	})

	return err
}

func (s *Svc) RetriveToken(ctx context.Context, key string) (oauth2.TokenSource, error) {
	var bytes []byte

	err := s.database.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(bucketTokens)
		if err != nil {
			return err
		}

		bytes = bucket.Get([]byte(key))
		return nil
	})
	if err != nil {
		return nil, err
	}

	var tok *oauth2.Token
	err = json.Unmarshal(bytes, tok)
	if err != nil {
		return nil, err
	}

	return s.oauthConfig.TokenSource(ctx, tok), nil
}

func (s *Svc) ForEach(ctx context.Context, fn func(context.Context, string, *calendar.Service) error) error {
	err := s.database.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(bucketTokens)
		if err != nil {
			return err
		}

		return bucket.ForEach(func(cID, bytes []byte) error {
			var tok *oauth2.Token
			err = json.Unmarshal(bytes, &tok)
			if err != nil {
				return err
			}

			tokenSource := s.oauthConfig.TokenSource(ctx, tok)
			cal, err := calendar.NewService(ctx, option.WithTokenSource(tokenSource))
			if err != nil {
				return err
			}

			return fn(ctx, string(cID), cal)
		})
	})
	return err
}

type Event struct {
	EventTitle string
	Starts     time.Time
	Ends       time.Time
	Attendees  []string
}

func (s *Svc) eventsByCalendar(ctx context.Context) (map[string][]Event, error) {
	events := map[string][]Event{}

	err := s.ForEach(ctx, func(ctx context.Context, cID string, svc *calendar.Service) error {
		es, err := svc.Events.List("primary").
			Context(ctx).
			TimeMin(time.Now().Format(time.RFC3339)).
			TimeMax(time.Now().AddDate(0, 0, 7).Format(time.RFC3339)).
			Do()
		if err != nil {
			log.Printf("[ERROR] failed to list all events for calendar '%s': %v\n", cID, err)
			return nil
		}

		items := []Event{}
		for _, item := range es.Items {
			attendees := []string{}
			for _, attendee := range item.Attendees {
				if attendee == nil {
					continue
				}
				if attendee.Self {
					continue
				}

				attendees = append(attendees, attendee.Email)
			}

			if item.Start.DateTime == "" {
				continue
			}

			startTime, err := time.Parse(time.RFC3339, item.Start.DateTime)
			if err != nil {
				return err
			}

			var endTime time.Time
			if item.End.DateTime != "" {
				endTime, err = time.Parse(time.RFC3339, item.End.DateTime)
				if err != nil {
					return err
				}
			}

			items = append(items, Event{
				EventTitle: item.Summary,
				Starts:     startTime,
				Ends:       endTime,
				Attendees:  attendees,
			})
			sort.Slice(items, func(i, j int) bool {
				return items[i].Starts.Before(items[j].Starts)
			})

			events[cID] = items
		}
		return nil
	})

	return events, err
}

func (s *Svc) BackgroundJob(ctx context.Context) {
	updateTicker := time.NewTicker(10 * time.Minute)
	recountTicker := time.NewTicker(time.Second)

	events, err := s.eventsByCalendar(ctx)
	if err != nil {
		log.Fatalf("[FATAL] failed to update events on boot: %v\n", err)
	} else {
		log.Printf("successfully updated events\n")
	}
	s.updatePrometheusGauges(events)

	for {
		select {
		case <-s.manualUpdate:
			events, err = s.eventsByCalendar(ctx)
			if err != nil {
				log.Printf("[ERROR] failed to update events cache: %v\n", err)
			} else {
				log.Printf("manual update successful\n")
			}
		case <-updateTicker.C:
			events, err = s.eventsByCalendar(ctx)
			if err != nil {
				log.Printf("[ERROR] failed to update events cache: %v\n", err)
			} else {
				log.Printf("timed update successful\n")
			}
		case <-recountTicker.C:
			s.updatePrometheusGauges(events)
		case <-ctx.Done():
			updateTicker.Stop()
			recountTicker.Stop()
			return
		}
	}
}

func (s *Svc) updatePrometheusGauges(calendarEvents map[string][]Event) {
	for cal, events := range calendarEvents {
		var updatedMeeting bool
		var updatedFocusTime bool
		const focusTime = "focusTime"
		const meeting = "meeting"

		for _, event := range events {
			if !event.Starts.After(time.Now().Add(-3 * time.Minute)) {
				continue
			}

			durationTilStart := event.Starts.Sub(time.Now()).Seconds()

			var kind string
			if len(event.Attendees) > 0 {
				if updatedMeeting {
					continue
				}
				updatedMeeting = true
				kind = meeting
			} else {
				if updatedFocusTime {
					continue
				}
				updatedFocusTime = true
				kind = focusTime
			}

			s.gaugeNextMeetingDuration.WithLabelValues(cal, kind).Set(durationTilStart)
			s.gaugeNextMeetingEpoch.WithLabelValues(cal, kind).Set(float64(event.Starts.Unix()))
			if updatedMeeting && updatedFocusTime {
				break
			}
		}

		if !updatedMeeting {
			s.gaugeNextMeetingDuration.DeleteLabelValues(cal, meeting)
			s.gaugeNextMeetingEpoch.DeleteLabelValues(cal, meeting)
		}
		if !updatedFocusTime {
			s.gaugeNextMeetingDuration.DeleteLabelValues(cal, focusTime)
			s.gaugeNextMeetingEpoch.DeleteLabelValues(cal, focusTime)
		}
	}
}
