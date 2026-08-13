package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"

	"github.com/carlosmaranje/gosolar"
	coretools "github.com/carlosmaranje/mango/core/tools"
)

func decimalHoursToTime(decimalHours float64) string {
	hours := int(decimalHours)
	minutes := int(math.Round((decimalHours - float64(hours)) * 60))
	if minutes == 60 {
		hours++
		minutes = 0
	}
	ampm := "AM"
	displayHour := hours
	if hours >= 12 {
		ampm = "PM"
		if hours > 12 {
			displayHour = hours - 12
		}
	}
	return fmt.Sprintf("%d:%02d %s", displayHour, minutes, ampm)
}

type GoSolarTool struct{}

func (t *GoSolarTool) Name() string {
	return "gosolar"
}

func (t *GoSolarTool) Description() string {
	return "Calculate solar position, angles, and timing data for a given location and date. Useful for solar energy planning and any solar related calculations."
}

func (t *GoSolarTool) Returns() string {
	return coretools.DescribeReturnType(SolarResult{})
}

func (t *GoSolarTool) Parameters() []coretools.Parameter {
	return []coretools.Parameter{
		{
			Name:        "latitude",
			Type:        "number",
			Description: "Latitude coordinate in degrees (-90 to 90)",
			Required:    true,
		},
		{
			Name:        "longitude",
			Type:        "number",
			Description: "Longitude coordinate in degrees (-180 to 180)",
			Required:    true,
		},
		{
			Name:        "date",
			Type:        "string",
			Description: "Date in YYYY-MM-DD format",
			Required:    true,
		},
		{
			Name:        "timeZone",
			Type:        "string",
			Description: "IANA timezone (e.g., 'UTC', 'America/New_York'), optional; defaults to UTC",
			Required:    false,
		},
		{
			Name:        "dayTime",
			Type:        "number",
			Description: "Decimal fraction of day (0-1) for the calculation time, optional; defaults to 0.5 (solar noon). Example: 0.25 = 6am, 0.5 = noon, 0.75 = 6pm",
			Required:    false,
		},
	}
}

type GoSolarInput struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Date      string  `json:"date"`
	TimeZone  string  `json:"timeZone"`
	DayTime   float64 `json:"dayTime"`
}

type SolarResult struct {
	Location struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"location"`
	Date       string  `json:"date"`
	TimeZone   string  `json:"timeZone"`
	DayTime    float64 `json:"dayTime"`
	TimingData struct {
		SolarNoon              float64 `json:"solarNoon"`
		SolarNoonFormatted     string  `json:"solarNoonFormatted"`
		SunriseTime            float64 `json:"sunriseTime"`
		SunriseTimeFormatted   string  `json:"sunriseTimeFormatted"`
		SunsetTime             float64 `json:"sunsetTime"`
		SunsetTimeFormatted    string  `json:"sunsetTimeFormatted"`
		DayLength              float64 `json:"dayLength"`
		DayLengthFormatted     string  `json:"dayLengthFormatted"`
		TrueSolarTime          float64 `json:"trueSolarTime"`
		TrueSolarTimeFormatted string  `json:"trueSolarTimeFormatted"`
	} `json:"timingData"`
	AngularData struct {
		SolarZenithAngle    float64 `json:"solarZenithAngle"`
		SolarAzimuthAngle   float64 `json:"solarAzimuthAngle"`
		SolarIncidenceAngle float64 `json:"solarIncidenceAngle"`
		SolarDeclination    float64 `json:"solarDeclination"`
		SunHourAngle        float64 `json:"sunHourAngle"`
		HourAngleSunrise    float64 `json:"hourAngleSunrise"`
	} `json:"angularData"`
	SolarCoordinates struct {
		EquationOfTime       float64 `json:"equationOfTime"`
		SunTrueLongitude     float64 `json:"sunTrueLongitude"`
		SunApparentLongitude float64 `json:"sunApparentLongitude"`
		SunEquationOfCenter  float64 `json:"sunEquationOfCenter"`
		GeomMeanLongSun      float64 `json:"geomMeanLongSun"`
		GeomMeanAnomSun      float64 `json:"geomMeanAnomSun"`
	} `json:"solarCoordinates"`
	OrbitalData struct {
		JulianDay         float64 `json:"julianDay"`
		JulianCentury     float64 `json:"julianCentury"`
		EccentEarthOrbit  float64 `json:"eccentEarthOrbit"`
		MeanObliqEcliptic float64 `json:"meanObliqEcliptic"`
		ObliqueCorrection float64 `json:"obliqueCorrection"`
	} `json:"orbitalData"`
	TimeZoneOffset float64 `json:"timeZoneOffset"`
}

func (t *GoSolarTool) Execute(ctx context.Context, input string) (string, error) {
	var req GoSolarInput
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if req.Latitude < -90 || req.Latitude > 90 {
		return "", fmt.Errorf("latitude must be between -90 and 90")
	}
	if req.Longitude < -180 || req.Longitude > 180 {
		return "", fmt.Errorf("longitude must be between -180 and 180")
	}

	if req.TimeZone == "" {
		req.TimeZone = "UTC"
	}
	if req.DayTime == 0 {
		req.DayTime = 0.5
	}
	if req.DayTime < 0 || req.DayTime > 1 {
		return "", fmt.Errorf("dayTime must be between 0 and 1 (got %f)", req.DayTime)
	}

	log.Printf("gosolar: calculating for lat=%f lon=%f date=%s tz=%s dayTime=%.4f (fraction of day)",
		req.Latitude, req.Longitude, req.Date, req.TimeZone, req.DayTime)

	calc, err := gosolar.Calculator(req.Latitude, req.Longitude, req.DayTime, req.TimeZone, req.Date)
	if err != nil {
		return "", fmt.Errorf("gosolar calculator error: %w", err)
	}

	sunrise, sunset := calc.SunriseAndSunset()
	result := SolarResult{
		Location: struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		}{Latitude: req.Latitude, Longitude: req.Longitude},
		Date:     req.Date,
		TimeZone: req.TimeZone,
		DayTime:  req.DayTime,
	}

	result.TimingData.SolarNoon = calc.SolarNoon()
	result.TimingData.SolarNoonFormatted = decimalHoursToTime(calc.SolarNoon())
	result.TimingData.SunriseTime = sunrise
	result.TimingData.SunriseTimeFormatted = decimalHoursToTime(sunrise)
	result.TimingData.SunsetTime = sunset
	result.TimingData.SunsetTimeFormatted = decimalHoursToTime(sunset)
	result.TimingData.DayLength = calc.DayLength()
	result.TimingData.DayLengthFormatted = fmt.Sprintf("%.2f hours", calc.DayLength())
	result.TimingData.TrueSolarTime = calc.TrueSolarTime()
	result.TimingData.TrueSolarTimeFormatted = decimalHoursToTime(calc.TrueSolarTime())

	result.AngularData.SolarZenithAngle = calc.SolarZenithAngle()
	result.AngularData.SolarAzimuthAngle = calc.SolarAzimuthAngle()
	result.AngularData.SolarIncidenceAngle = calc.SolarIncidenceAngle()
	result.AngularData.SolarDeclination = calc.SolarDeclination()
	result.AngularData.SunHourAngle = calc.SunHourAngle()
	result.AngularData.HourAngleSunrise = calc.HourAngleSunrise()

	result.SolarCoordinates.EquationOfTime = calc.EquationOfTime()
	result.SolarCoordinates.SunTrueLongitude = calc.SunTrueLongitude()
	result.SolarCoordinates.SunApparentLongitude = calc.SunApparentLongitude()
	result.SolarCoordinates.SunEquationOfCenter = calc.SunEquationOfCenter()
	result.SolarCoordinates.GeomMeanLongSun = calc.GeomMeanLongSun()
	result.SolarCoordinates.GeomMeanAnomSun = calc.GeomMeanAnomSun()

	result.OrbitalData.JulianDay = calc.JulianDay()
	result.OrbitalData.JulianCentury = calc.JulianCentury()
	result.OrbitalData.EccentEarthOrbit = calc.EccentEarthOrbit()
	result.OrbitalData.MeanObliqEcliptic = calc.MeanObliqEcliptic()
	result.OrbitalData.ObliqueCorrection = calc.ObliqueCorrection()

	result.TimeZoneOffset = calc.GetTimeZoneOffset()

	output, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(output), nil
}

func NewGoSolarTool() *GoSolarTool {
	return &GoSolarTool{}
}
