# Google Calendar exporter for Prometheus

_eh, it's not ready_

This is a small Prometheus exporter that lets users authenticate to export
the number of seconds until their next event in Google Calendar divided by
focus time (events just for you) and meetings (events with other attendees).

```
calendar_next_meeting_seconds{calendar="<calendarID>",type="focusTime"} 309773.160531
calendar_next_meeting_seconds{calendar="<calendarID>",type="meeting"} 173.160545
```

It's possible that I'll finish and document this at some stage, but this is
just a code dump at the moment.

## Getting ready

1. Install gcal-exporter using `go install github.com/dememorized/gcal-exporter@latest`.
2. Create a directory somewhere for `gcal-exporter` to run in, such as `/opt/gcal-exporter`. This will be your working
directory. Within that directory, create a directory called `data`.
3. Go to [console.cloud.google.com/apis/credentials](https://console.cloud.google.com/apis/credentials) and get
OAuth2 web application credentials. You need to have the calendar API enabled, but you should get a friendly error with
a URL when you try to start if you don't. Download the JSON for the credentials and save to your data directory as
`google.json` (you can also set the path using the command-line option `-goog.credentials=/path/to/client-certificate.json`)
4. Start the application from your working directory.
5. Go to http://localhost:8080/auth and authenticate with Google Calendar. In case something goes wrong, you should get
an error in the application logs to help you figure out how to fix it.
6. Go to http://localhost:8080/metrics, you should see a few metrics for your calendar.

## Licensing

Copyright 2022 Emil Tullstedt.<br/>
Licensed under the EUPL.
