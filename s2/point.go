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
	"math"

	"github.com/golang/geo/r3"
	"github.com/golang/geo/s1"
)

// Direction is an indication of the ordering of a set of points
type Direction int

// These are the three options for the direction of a set of points.
const (
	Clockwise        Direction = -1
	Indeterminate              = 0
	CounterClockwise           = 1
)

// maxDeterminantError is the maximum error in computing (AxB).C where all vectors
// are unit length. Using standard inequalities, it can be shown that
//
//  fl(AxB) = AxB + D where |D| <= (|AxB| + (2/sqrt(3))*|A|*|B|) * e
//
// where "fl()" denotes a calculation done in floating-point arithmetic,
// |x| denotes either absolute value or the L2-norm as appropriate, and
// e is a reasonably small value near the noise level of floating point
// number accuracy. Similarly,
//
//  fl(B.C) = B.C + d where |d| <= (|B.C| + 2*|B|*|C|) * e .
//
// Applying these bounds to the unit-length vectors A,B,C and neglecting
// relative error (which does not affect the sign of the result), we get
//
//  fl((AxB).C) = (AxB).C + d where |d| <= (3 + 2/sqrt(3)) * e
const maxDeterminantError = 4.6125e-16

// detErrorMultiplier is the factor to scale the magnitudes by when checking
// for the sign of set of points with certainty. Using a similar technique to
// the one used for maxDeterminantError, the error is at most:
//
//   |d| <= (3 + 6/sqrt(3)) * |A-C| * |B-C| * e
//
// If the determinant magnitude is larger than this value then we know its sign with certainty.
const detErrorMultiplier = 7.1767e-16

// Point represents a point on the unit sphere as a normalized 3D vector.
//
// Points are guaranteed to be close to normal in the sense that the norm of any points will be very close to 1.
//
// Fields should be treated as read-only. Use one of the factory methods for creation.
type Point struct {
	r3.Vector
}

// PointFromCoords creates a new normalized point from coordinates.
//
// This always returns a valid point. If the given coordinates can not be normalized
// the origin point will be returned.
//
// This behavior is different from the C++ construction of a S2Point from coordinates
// (i.e. S2Point(x, y, z)) in that in C++ they do not Normalize.
func PointFromCoords(x, y, z float64) Point {
	if x == 0 && y == 0 && z == 0 {
		return OriginPoint()
	}
	return Point{r3.Vector{x, y, z}.Normalize()}
}

// PointFromLatLng returns an Point for the given LatLng.
func PointFromLatLng(ll LatLng) Point {
	phi := ll.Lat.Radians()
	theta := ll.Lng.Radians()
	cosphi := math.Cos(phi)
	return PointFromCoords(math.Cos(theta)*cosphi, math.Sin(theta)*cosphi, math.Sin(phi))
}

// OriginPoint returns a unique "origin" on the sphere for operations that need a fixed
// reference point. In particular, this is the "point at infinity" used for
// point-in-polygon testing (by counting the number of edge crossings).
//
// It should *not* be a point that is commonly used in edge tests in order
// to avoid triggering code to handle degenerate cases (this rules out the
// north and south poles). It should also not be on the boundary of any
// low-level S2Cell for the same reason.
func OriginPoint() Point {
	return Point{r3.Vector{0.00456762077230, 0.99947476613078, 0.03208315302933}}
}

// PointCross returns a Point that is orthogonal to both p and op. This is similar to
// p.Cross(op) (the true cross product) except that it does a better job of
// ensuring orthogonality when the Point is nearly parallel to op, it returns
// a non-zero result even when p == op or p == -op and the result is a Point,
// so it will have norm 1.
//
// It satisfies the following properties (f == PointCross):
//
//   (1) f(p, op) != 0 for all p, op
//   (2) f(op,p) == -f(p,op) unless p == op or p == -op
//   (3) f(-p,op) == -f(p,op) unless p == op or p == -op
//   (4) f(p,-op) == -f(p,op) unless p == op or p == -op
func (p Point) PointCross(op Point) Point {
	// NOTE(dnadasi): In the C++ API the equivalent method here was known as "RobustCrossProd",
	// but PointCross more accurately describes how this method is used.
	x := p.Add(op.Vector).Cross(op.Sub(p.Vector))

	if x.ApproxEqual(r3.Vector{0, 0, 0}) {
		// The only result that makes sense mathematically is to return zero, but
		// we find it more convenient to return an arbitrary orthogonal vector.
		return Point{p.Ortho()}
	}

	return Point{x.Normalize()}
}

// CCW returns true if the points A, B, C are strictly counterclockwise,
// and returns false if the points are clockwise or collinear (i.e. if they are all
// contained on some great circle).
//
// Due to numerical errors, situations may arise that are mathematically
// impossible, e.g. ABC may be considered strictly CCW while BCA is not.
// However, the implementation guarantees the following:
//
//   If CCW(a,b,c), then !CCW(c,b,a) for all a,b,c.
func CCW(a, b, c Point) bool {
	// NOTE(dnadasi): In the C++ API the equivalent method here was known as "SimpleCCW",
	// but CCW seems like a fine name at least until the need for a RobustCCW is demonstrated.

	// We compute the signed volume of the parallelepiped ABC. The usual
	// formula for this is (A ⨯ B) · C, but we compute it here using (C ⨯ A) · B
	// in order to ensure that ABC and CBA are not both CCW. This follows
	// from the following identities (which are true numerically, not just
	// mathematically):
	//
	//     (1) x ⨯ y == -(y ⨯ x)
	//     (2) -x · y == -(x · y)
	return c.Cross(a.Vector).Dot(b.Vector) > 0
}

// RobustSign returns a Direction representing the ordering of the points.
// CounterClockwise is returned if the points are in counter-clockwise order,
// Clockwise for clockwise, and Indeterminate if any two points are the same (collinear),
// or the sign could not completely be determined.
//
// This function has additional logic to make sure that the above properties hold even
// when the three points are coplanar, and to deal with the limitations of
// floating-point arithmetic.
//
// RobustSign satisfies the following conditions:
//
//  (1) RobustSign(a,b,c) == 0 if and only if a == b, b == c, or c == a
//  (2) RobustSign(b,c,a) == RobustSign(a,b,c) for all a,b,c
//  (3) RobustSign(c,b,a) == -RobustSign(a,b,c) for all a,b,c
//
// In other words:
//
//  (1) The result is zero if and only if two points are the same.
//  (2) Rotating the order of the arguments does not affect the result.
//  (3) Exchanging any two arguments inverts the result.
//
// On the other hand, note that it is not true in general that
// RobustSign(-a,b,c) == -RobustSign(a,b,c), or any similar identities
// involving antipodal points.
//
// C++ Equivalent: RobustCCW()
func RobustSign(a, b, c Point) Direction {
	// This method combines the computations from the C++ methods
	// RobustCCW, TriageCCW, ExpensiveCCW, and StableCCW.
	// TODO: Split these extra functions out when the need arises.

	// Start with TriageCCW
	det := c.Cross(a.Vector).Dot(b.Vector)
	if det > maxDeterminantError {
		return CounterClockwise
	}
	if det < -maxDeterminantError {
		return Clockwise
	}

	// ExpensiveCCW
	if a == b || b == c || c == a {
		return Indeterminate
	}

	// StableCCW
	ab := a.Sub(b.Vector)
	ab2 := ab.Norm2()
	bc := b.Sub(c.Vector)
	bc2 := bc.Norm2()
	ca := c.Sub(a.Vector)
	ca2 := ca.Norm2()

	// The two shortest edges, pointing away from their common point.
	var e1, e2, op r3.Vector

	if ab2 >= bc2 && ab2 >= ca2 {
		// AB is the longest edge.
		e1, e2, op = ca, bc, c.Vector
	} else if bc2 >= ca2 {
		// BC is the longest edge.
		e1, e2, op = ab, ca, a.Vector
	} else {
		// CA is the longest edge.
		e1, e2, op = bc, ab, b.Vector
	}

	det = e1.Cross(e2).Dot(op)
	maxErr := detErrorMultiplier * math.Sqrt(e1.Norm2()*e2.Norm2())

	// If the determinant isn't zero, we know definitively the point ordering.
	if det > maxErr {
		return CounterClockwise
	}
	if det < -maxErr {
		return Clockwise
	}

	// In the C++ version, the final computation is performed using OpenSSL's
	// Bignum exact precision math library. The existence of an equivalent
	// library in Go is indeterminate. In C++, using the exact precision library
	// to solve this stage is ~300x slower than the above checks.
	// TODO(roberts): Select and incorporate an appropriate Go exact precision
	// floating point library for the remaining calculations.
	return Indeterminate
}

// OrderedCCW returns true if the edges OA, OB, and OC are encountered in that
// order while sweeping CCW around the point O.
//
// You can think of this as testing whether A <= B <= C with respect to the
// CCW ordering around O that starts at A, or equivalently, whether B is
// contained in the range of angles (inclusive) that starts at A and extends
// CCW to C. Properties:
//
//  (1) If OrderedCCW(a,b,c,o) && OrderedCCW(b,a,c,o), then a == b
//  (2) If OrderedCCW(a,b,c,o) && OrderedCCW(a,c,b,o), then b == c
//  (3) If OrderedCCW(a,b,c,o) && OrderedCCW(c,b,a,o), then a == b == c
//  (4) If a == b or b == c, then OrderedCCW(a,b,c,o) is true
//  (5) Otherwise if a == c, then OrderedCCW(a,b,c,o) is false
func OrderedCCW(a, b, c, o Point) bool {
	sum := 0
	if RobustSign(b, o, a) != Clockwise {
		sum++
	}
	if RobustSign(c, o, b) != Clockwise {
		sum++
	}
	if RobustSign(a, o, c) == CounterClockwise {
		sum++
	}
	return sum >= 2
}

// Distance returns the angle between two points.
func (p Point) Distance(b Point) s1.Angle {
	return p.Vector.Angle(b.Vector)
}

// ApproxEqual reports if the two points are similar enough to be equal.
func (p Point) ApproxEqual(other Point) bool {
	const epsilon = 1e-14
	return p.Vector.Angle(other.Vector) <= epsilon
}

// PointArea returns the area on the unit sphere for the triangle defined by the
// given points.
//
// This method is based on l'Huilier's theorem,
//
//   tan(E/4) = sqrt(tan(s/2) tan((s-a)/2) tan((s-b)/2) tan((s-c)/2))
//
// where E is the spherical excess of the triangle (i.e. its area),
//       a, b, c are the side lengths, and
//       s is the semiperimeter (a + b + c) / 2.
//
// The only significant source of error using l'Huilier's method is the
// cancellation error of the terms (s-a), (s-b), (s-c). This leads to a
// *relative* error of about 1e-16 * s / min(s-a, s-b, s-c). This compares
// to a relative error of about 1e-15 / E using Girard's formula, where E is
// the true area of the triangle. Girard's formula can be even worse than
// this for very small triangles, e.g. a triangle with a true area of 1e-30
// might evaluate to 1e-5.
//
// So, we prefer l'Huilier's formula unless dmin < s * (0.1 * E), where
// dmin = min(s-a, s-b, s-c). This basically includes all triangles
// except for extremely long and skinny ones.
//
// Since we don't know E, we would like a conservative upper bound on
// the triangle area in terms of s and dmin. It's possible to show that
// E <= k1 * s * sqrt(s * dmin), where k1 = 2*sqrt(3)/Pi (about 1).
// Using this, it's easy to show that we should always use l'Huilier's
// method if dmin >= k2 * s^5, where k2 is about 1e-2. Furthermore,
// if dmin < k2 * s^5, the triangle area is at most k3 * s^4, where
// k3 is about 0.1. Since the best case error using Girard's formula
// is about 1e-15, this means that we shouldn't even consider it unless
// s >= 3e-4 or so.
func PointArea(a, b, c Point) float64 {
	sa := float64(b.Angle(c.Vector))
	sb := float64(c.Angle(a.Vector))
	sc := float64(a.Angle(b.Vector))
	s := 0.5 * (sa + sb + sc)
	if s >= 3e-4 {
		// Consider whether Girard's formula might be more accurate.
		dmin := s - math.Max(sa, math.Max(sb, sc))
		if dmin < 1e-2*s*s*s*s*s {
			// This triangle is skinny enough to use Girard's formula.
			ab := a.PointCross(b)
			bc := b.PointCross(c)
			ac := a.PointCross(c)
			area := math.Max(0.0, float64(ab.Angle(ac.Vector)-ab.Angle(bc.Vector)+bc.Angle(ac.Vector)))

			if dmin < s*0.1*area {
				return area
			}
		}
	}

	// Use l'Huilier's formula.
	return 4 * math.Atan(math.Sqrt(math.Max(0.0, math.Tan(0.5*s)*math.Tan(0.5*(s-sa))*
		math.Tan(0.5*(s-sb))*math.Tan(0.5*(s-sc)))))
}

// TODO(dnadasi):
//   - Other CCW methods
//   - Maybe more Area methods?
//   - Centroid methods
