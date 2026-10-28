// Package engine works out where shade falls on a street through a hot day, and which trees,
// planted where, would shade the most people when it matters most.
package engine

import "math"

const (
	deg = math.Pi / 180
	rad = 180 / math.Pi
)

// Sun is the sun's position: elevation above the horizon and azimuth clockwise from north,
// both in degrees.
type Sun struct {
	Elevation float64
	Azimuth   float64
}

// julianDay returns the Julian day number for a calendar date at 0h UT.
func julianDay(year, month, day int) float64 {
	if month <= 2 {
		year--
		month += 12
	}
	a := year / 100
	b := 2 - a + a/4
	return math.Floor(365.25*float64(year+4716)) + math.Floor(30.6001*float64(month+1)) + float64(day) + float64(b) - 1524.5
}

// SunAt returns the sun's position using the NOAA solar calculator equations (accurate to well
// under a degree for 1900–2100, which is far finer than a street needs).
//
// lat and lon are in degrees (south and west negative), tz is the clock's offset from UTC in hours,
// and hour is the local clock time in hours (e.g. 15.5 for 3:30 pm).
func SunAt(lat, lon, tz float64, year, month, day int, hour float64) Sun {
	jd := julianDay(year, month, day) + (hour-tz)/24
	t := (jd - 2451545) / 36525 // Julian centuries since J2000

	l0 := math.Mod(280.46646+t*(36000.76983+t*0.0003032), 360) // mean longitude
	m := 357.52911 + t*(35999.05029-0.0001537*t)               // mean anomaly
	e := 0.016708634 - t*(0.000042037+0.0000001267*t)          // orbit eccentricity
	c := math.Sin(m*deg)*(1.914602-t*(0.004817+0.000014*t)) + math.Sin(2*m*deg)*(0.019993-0.000101*t) +
		math.Sin(3*m*deg)*0.000289 // equation of centre
	trueLong := l0 + c
	omega := 125.04 - 1934.136*t
	lambda := trueLong - 0.00569 - 0.00478*math.Sin(omega*deg) // apparent longitude
	eps0 := 23 + (26+(21.448-t*(46.815+t*(0.00059-t*0.001813)))/60)/60
	eps := eps0 + 0.00256*math.Cos(omega*deg) // obliquity, corrected
	decl := math.Asin(math.Sin(eps*deg)*math.Sin(lambda*deg)) * rad

	y := math.Pow(math.Tan(eps/2*deg), 2)
	eqTime := 4 * rad * (y*math.Sin(2*l0*deg) - 2*e*math.Sin(m*deg) + 4*e*y*math.Sin(m*deg)*math.Cos(2*l0*deg) -
		0.5*y*y*math.Sin(4*l0*deg) - 1.25*e*e*math.Sin(2*m*deg)) // minutes

	trueSolarTime := math.Mod(hour*60+eqTime+4*lon-60*tz, 1440)
	if trueSolarTime < 0 {
		trueSolarTime += 1440
	}
	hourAngle := trueSolarTime/4 - 180

	cosZenith := math.Sin(lat*deg)*math.Sin(decl*deg) + math.Cos(lat*deg)*math.Cos(decl*deg)*math.Cos(hourAngle*deg)
	cosZenith = math.Max(-1, math.Min(1, cosZenith))
	zenith := math.Acos(cosZenith) * rad
	elev := 90 - zenith
	elev += refraction(elev)

	// Azimuth clockwise from north.
	den := math.Cos(lat*deg) * math.Sin(zenith*deg)
	var az float64
	if math.Abs(den) < 1e-9 {
		az = 180
		if lat < 0 {
			az = 0
		}
	} else {
		ca := (math.Sin(lat*deg)*cosZenith - math.Sin(decl*deg)) / den
		ca = math.Max(-1, math.Min(1, ca))
		az = math.Acos(ca) * rad
		if hourAngle > 0 {
			az = math.Mod(az+180, 360)
		} else {
			az = math.Mod(540-az, 360)
		}
	}
	return Sun{Elevation: elev, Azimuth: az}
}

// refraction is the NOAA approximation of atmospheric refraction, in degrees.
func refraction(elev float64) float64 {
	if elev > 85 {
		return 0
	}
	te := math.Tan(elev * deg)
	var r float64 // arc seconds
	switch {
	case elev > 5:
		r = 58.1/te - 0.07/(te*te*te) + 0.000086/math.Pow(te, 5)
	case elev > -0.575:
		r = 1735 + elev*(-518.2+elev*(103.4+elev*(-12.79+elev*0.711)))
	default:
		r = -20.774 / te
	}
	return r / 3600
}

// DirectBeam is a clear-sky estimate of direct sunlight in W/m² facing the sun (Meinel's
// air-mass model). It is zero with the sun below 1° and grows quickly as the sun climbs.
func DirectBeam(elevation float64) float64 {
	if elevation <= 1 {
		return 0
	}
	am := 1 / math.Sin(elevation*deg)
	return 1353 * math.Pow(0.7, math.Pow(am, 0.678))
}
