package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type DateTimeTool struct{}

func (d *DateTimeTool) Name() string { return "datetime" }

func (d *DateTimeTool) Description() string {
	return `Returns current date/time and calendar information. Use to know today's date, 
		find the day of week for any date, calculate how many days until a given weekday,
		or measure the distance in days between two dates. If the user mentions a relative day (e.g., 'next Tuesday') 
		and you are unsure of the date, call this tool first to resolve the calendar context`
}

func (d *DateTimeTool) Parameters() []Parameter {
	return []Parameter{
		{
			Name:        "timezone",
			Type:        "string",
			Description: "IANA timezone (e.g. 'UTC', 'America/New_York'). Defaults to UTC.",
			Required:    false,
		},
		{
			Name:        "date",
			Type:        "string",
			Description: "Date to query in YYYY-MM-DD format. Defaults to today.",
			Required:    false,
		},
	}
}

type DateTimeInput struct {
	Timezone string `json:"timezone"`
	Date     string `json:"date"`
}

type Weekday struct {
	Monday    int `json:"monday"`
	Tuesday   int `json:"tuesday"`
	Wednesday int `json:"wednesday"`
	Thursday  int `json:"thursday"`
	Friday    int `json:"friday"`
	Saturday  int `json:"saturday"`
	Sunday    int `json:"sunday"`
}

type DateTimeResult struct {
	Now           string  `json:"now"`
	Timezone      string  `json:"timezone"`
	QueriedDate   string  `json:"queried_date"`
	DayOfWeek     string  `json:"day_of_week"`
	WeekNumber    int     `json:"week_number"`
	YearDay       int     `json:"year_day"`
	DaysFromToday int     `json:"days_from_today"`
	DaysUntil     Weekday `json:"days_until_next_weekday"`
	IsWeekend     bool    `json:"is_weekend"`
	TimeOfTheDay  string  `json:"time_of_the_day"`
}

func (d *DateTimeTool) Returns() string {
	return DescribeReturnType(DateTimeResult{})
}

// daysUntil returns how many days from `from` until the next occurrence of `target` weekday.
// Always returns 1–7 (never 0 — "next Monday" from Monday is 7 days away).
func daysUntil(from time.Time, target time.Weekday) int {
	diff := int(target) - int(from.Weekday())
	if diff <= 0 {
		diff += 7
	}
	return diff
}

func timeOfTheDay(time time.Time) string {
	hour := time.Hour()
	switch {
	case hour > 0 && hour <= 6:
		return "Late night / small hours"
	case hour > 6 && hour <= 12:
		return "Morning"
	case hour > 12 && hour <= 18:
		return "Afternoon"
	case hour > 18 && hour <= 23:
		return "Night"
	}
	return ""
}

func (d *DateTimeTool) Execute(_ context.Context, input string) (string, error) {
	var req DateTimeInput
	if input != "" && input != "{}" {
		if err := json.Unmarshal([]byte(input), &req); err != nil {
			return "", fmt.Errorf("invalid input: %w", err)
		}
	}

	if req.Timezone == "" {
		req.Timezone = "UTC"
	}
	loc, err := time.LoadLocation(req.Timezone)
	if err != nil {
		return "", fmt.Errorf("unknown timezone %q: %w", req.Timezone, err)
	}

	now := time.Now().In(loc)

	var queried time.Time
	if req.Date == "" {
		queried = now
	} else {
		switch req.Date {
		case "today":
			queried = now
		case "tomorrow":
			queried = now.AddDate(0, 0, 1)
		case "yesterday":
			queried = now.AddDate(0, 0, -1)
		default:
			queried, err = time.ParseInLocation("2006-01-02", req.Date, loc)
		}
		if err != nil {
			return "", fmt.Errorf("invalid date %q, expected YYYY-MM-DD: %w", req.Date, err)
		}
	}

	_, week := queried.ISOWeek()

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	queriedDay := time.Date(queried.Year(), queried.Month(), queried.Day(), 0, 0, 0, 0, loc)
	daysFromToday := int(queriedDay.Sub(today).Hours() / 24)

	result := DateTimeResult{
		Now:           now.Format(time.RFC3339),
		Timezone:      req.Timezone,
		QueriedDate:   queried.Format("2006-01-02"),
		DayOfWeek:     queried.Weekday().String(),
		WeekNumber:    week,
		YearDay:       queried.YearDay(),
		DaysFromToday: daysFromToday,
		DaysUntil: Weekday{
			Monday:    daysUntil(queried, time.Monday),
			Tuesday:   daysUntil(queried, time.Tuesday),
			Wednesday: daysUntil(queried, time.Wednesday),
			Thursday:  daysUntil(queried, time.Thursday),
			Friday:    daysUntil(queried, time.Friday),
			Saturday:  daysUntil(queried, time.Saturday),
			Sunday:    daysUntil(queried, time.Sunday),
		},
		IsWeekend:    queried.Weekday() == time.Sunday || queried.Weekday() == time.Saturday,
		TimeOfTheDay: timeOfTheDay(queried),
	}

	out, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}
	return string(out), nil
}

func NewDateTimeTool() *DateTimeTool {
	return &DateTimeTool{}
}
