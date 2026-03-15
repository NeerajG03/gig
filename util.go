package gig

import (
	"encoding/json"
	"time"
)

const timeFormat = time.RFC3339

func timeNowUTC() time.Time {
	return time.Now().UTC()
}

func labelsToJSON(labels []string) string {
	if labels == nil {
		labels = []string{}
	}
	b, _ := json.Marshal(labels)
	return string(b)
}

func labelsFromJSON(s string) []string {
	if s == "" || s == "[]" {
		return nil
	}
	var labels []string
	_ = json.Unmarshal([]byte(s), &labels)
	return labels
}

func timeToStr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(timeFormat)
}

func strToTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(timeFormat, s)
	if err != nil {
		return nil
	}
	return &t
}

// strPtr returns a pointer to s.
func strPtr(s string) *string {
	return &s
}
