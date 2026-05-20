package api

import (
	"fmt"
	"time"

	"github.com/leandrofars/oktopus/internal/db"
)

// campaignNowUTC is overridden in tests.
var campaignNowUTC = func() time.Time { return time.Now().UTC() }

func campaignHasTimeWindow(c db.Campaign) bool {
	return c.TimeWindowStart != "" && c.TimeWindowEnd != ""
}

// campaignWindowOccurrenceKey returns a stable key for the current UTC window period, or "" if outside the window.
func campaignWindowOccurrenceKey(c db.Campaign, now time.Time) string {
	if !campaignHasTimeWindow(c) {
		return ""
	}
	if !isWithinTimeWindowAt(c, now) {
		return ""
	}

	startMinutes, err := parseTimeHHMM(c.TimeWindowStart)
	if err != nil {
		return ""
	}
	endMinutes, err := parseTimeHHMM(c.TimeWindowEnd)
	if err != nil {
		return ""
	}

	periodDate := now.UTC().Format("2006-01-02")
	currentMinutes := now.UTC().Hour()*60 + now.UTC().Minute()

	if startMinutes <= endMinutes {
		return fmt.Sprintf("%s:%s:%s", periodDate, c.TimeWindowStart, c.TimeWindowEnd)
	}

	// Overnight window: before end belongs to period that started yesterday.
	if currentMinutes < endMinutes {
		periodDate = now.UTC().AddDate(0, 0, -1).Format("2006-01-02")
	}
	return fmt.Sprintf("%s:%s:%s", periodDate, c.TimeWindowStart, c.TimeWindowEnd)
}

func isWithinTimeWindowAt(campaign db.Campaign, now time.Time) bool {
	if campaign.TimeWindowStart == "" || campaign.TimeWindowEnd == "" {
		return true
	}

	currentMinutes := now.UTC().Hour()*60 + now.UTC().Minute()

	startMinutes, err := parseTimeHHMM(campaign.TimeWindowStart)
	if err != nil {
		return true
	}
	endMinutes, err := parseTimeHHMM(campaign.TimeWindowEnd)
	if err != nil {
		return true
	}

	if startMinutes <= endMinutes {
		return currentMinutes >= startMinutes && currentMinutes < endMinutes
	}
	return currentMinutes >= startMinutes || currentMinutes < endMinutes
}

func isWithinTimeWindow(campaign db.Campaign) bool {
	return isWithinTimeWindowAt(campaign, campaignNowUTC())
}
