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

Copyright 2022 Emil Tullstedt.<br/>
Licensed under the EUPL.
