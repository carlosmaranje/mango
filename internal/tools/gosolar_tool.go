package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/carlosmaranje/gosolar"
)

type GoSolarTool struct{}

func (t *GoSolarTool) Name() string {
	return "gosolar"
}

func (t *GoSolarTool) Description() string {
	return "Calculate solar position, angles, and timing data for a given location and date. Useful for solar energy planning and any solar related calculations."
}

func (t *GoSolarTool) Returns() string {
	return DescribeReturnType(SolarResult{})
}

func (t *GoSolarTool) Parameters() []Parameter {
	return []Parameter{
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
			Description: "Decimal hour (0-24) for the calculation time, optional; defaults to 12 (solar noon)",
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
		SolarNoon     float64 `json:"solarNoon"`
		SunriseTime   float64 `json:"sunriseTime"`
		SunsetTime    float64 `json:"sunsetTime"`
		DayLength     float64 `json:"dayLength"`
		TrueSolarTime float64 `json:"trueSolarTime"`
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
		req.DayTime = 12
	}

	log.Printf("gosolar: calculating for lat=%f lon=%f date=%s tz=%s dayTime=%f",
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
	result.TimingData.SunriseTime = sunrise
	result.TimingData.SunsetTime = sunset
	result.TimingData.DayLength = calc.DayLength()
	result.TimingData.TrueSolarTime = calc.TrueSolarTime()

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
