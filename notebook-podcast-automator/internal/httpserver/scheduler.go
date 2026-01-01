package httpserver

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

func (s *Server) startSchedulerIfConfigured() {
	at := strings.TrimSpace(os.Getenv("NPA_SCHEDULE_AT"))
	if at == "" {
		return
	}

	loc := time.Local
	if tz := strings.TrimSpace(os.Getenv("NPA_SCHEDULE_TZ")); tz != "" {
		if loaded, err := time.LoadLocation(tz); err == nil {
			loc = loaded
		} else {
			log.Printf("[schedule] invalid NPA_SCHEDULE_TZ=%s: %v", tz, err)
		}
	} else if tz := strings.TrimSpace(os.Getenv("NPA_TZ")); tz != "" {
		if loaded, err := time.LoadLocation(tz); err == nil {
			loc = loaded
		} else {
			log.Printf("[schedule] invalid NPA_TZ=%s: %v", tz, err)
		}
	}

	h, m, sec, err := parseClockAt(at)
	if err != nil {
		log.Printf("[schedule] invalid NPA_SCHEDULE_AT=%s: %v", at, err)
		return
	}

	log.Printf("[schedule] enabled at %02d:%02d:%02d (%s)", h, m, sec, loc.String())

	go func() {
		for {
			now := time.Now().In(loc)
			next := time.Date(now.Year(), now.Month(), now.Day(), h, m, sec, 0, loc)
			if !next.After(now) {
				next = next.Add(24 * time.Hour)
			}

			sleep := time.Until(next)
			if sleep > 0 {
				timer := time.NewTimer(sleep)
				<-timer.C
				timer.Stop()
			}

			if !s.runMu.TryLock() {
				log.Printf("[schedule] skipped: already_running")
				continue
			}
			_, _, runErr := s.executeRun(context.Background(), RunRequest{}, func(stage, message string) {
				log.Printf("[workflow] %s: %s", stage, message)
			})
			s.runMu.Unlock()

			if runErr != nil {
				log.Printf("[schedule] run failed: %v", runErr)
			} else {
				log.Printf("[schedule] run finished")
			}
		}
	}()
}

func parseClockAt(raw string) (hour, minute, second int, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0, 0, nil
	}

	parts := strings.Split(raw, ":")
	if len(parts) != 2 && len(parts) != 3 {
		return 0, 0, 0, fmtClockErr(raw)
	}

	h, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, 0, fmtClockErr(raw)
	}
	m, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, 0, fmtClockErr(raw)
	}
	s := 0
	if len(parts) == 3 {
		s, err = strconv.Atoi(strings.TrimSpace(parts[2]))
		if err != nil {
			return 0, 0, 0, fmtClockErr(raw)
		}
	}

	if h < 0 || h > 23 || m < 0 || m > 59 || s < 0 || s > 59 {
		return 0, 0, 0, fmtClockErr(raw)
	}
	return h, m, s, nil
}

func fmtClockErr(raw string) error {
	return &clockParseError{raw: raw}
}

type clockParseError struct {
	raw string
}

func (e *clockParseError) Error() string {
	return "expected HH:MM or HH:MM:SS"
}
