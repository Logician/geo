/*
Copyright 2014 Google Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package s2

import (
	"fmt"
	"math"

	"github.com/golang/geo/s1"
)

// LatLng represents a point on the unit sphere as a pair of angles.
type LatLng struct {
	Lat, Lng s1.Angle
}

// LatLngFromDegrees returns a LatLng for the coordinates given in degrees.
func LatLngFromDegrees(lat, lng float64) LatLng {
	return LatLng{s1.Angle(lat) * s1.Degree, s1.Angle(lng) * s1.Degree}
}

// LatLngFromPoint returns an LatLng for a given Point.
func LatLngFromPoint(p Point) LatLng {
	return LatLng{latitude(p), longitude(p)}
}

// IsValid returns true iff the LatLng is normalized, with Lat ∈ [-π/2,π/2] and Lng ∈ [-π,π].
func (ll LatLng) IsValid() bool {
	return math.Abs(ll.Lat.Radians()) <= math.Pi/2 && math.Abs(ll.Lng.Radians()) <= math.Pi
}

func (ll LatLng) String() string { return fmt.Sprintf("[%v, %v]", ll.Lat, ll.Lng) }

// Distance returns the angle between two LatLngs.
func (ll LatLng) Distance(ll2 LatLng) s1.Angle {
	// Haversine formula, as used in C++ S2LatLng::GetDistance.
	lat1, lat2 := ll.Lat.Radians(), ll2.Lat.Radians()
	lng1, lng2 := ll.Lng.Radians(), ll2.Lng.Radians()
	dlat := math.Sin(0.5 * (lat2 - lat1))
	dlng := math.Sin(0.5 * (lng2 - lng1))
	x := dlat*dlat + dlng*dlng*math.Cos(lat1)*math.Cos(lat2)
	return s1.Angle(2*math.Atan2(math.Sqrt(x), math.Sqrt(math.Max(0, 1-x)))) * s1.Radian
}

func latitude(p Point) s1.Angle {
	return s1.Angle(math.Atan2(p.Z, math.Sqrt(p.X*p.X+p.Y*p.Y)))
}

func longitude(p Point) s1.Angle {
	return s1.Angle(math.Atan2(p.Y, p.X))
}

// BUG(dsymonds): The major differences from the C++ version are:
//   - normalization
