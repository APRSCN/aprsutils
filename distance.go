package aprsutils

import "math"

// CalculateDistance computes the distance between two coordinates using Vincenty inverse formula
func CalculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// WGS-84 ellipsoid parameters
	a := 6378137.0           // Semi-major axis in meters
	b := 6356752.314245      // Semi-minor axis in meters
	f := 1.0 / 298.257223563 // Flattening

	L := toRadians(lon1 - lon2)
	U1 := math.Atan((1 - f) * math.Tan(toRadians(lat1)))
	U2 := math.Atan((1 - f) * math.Tan(toRadians(lat2)))

	sinU1 := math.Sin(U1)
	cosU1 := math.Cos(U1)
	sinU2 := math.Sin(U2)
	cosU2 := math.Cos(U2)

	lambda := L
	lambdaP := math.Pi
	var sinSigma, cosSigma, sigma, cosSqAlpha, cos2SigmaM float64

	circleCount := 40
	for math.Abs(lambda-lambdaP) > 1e-12 && circleCount > 0 {
		circleCount--
		sinLambda := math.Sin(lambda)
		cosLambda := math.Cos(lambda)

		sinSigma = math.Sqrt((cosU2*sinLambda)*(cosU2*sinLambda) +
			(cosU1*sinU2-sinU1*cosU2*cosLambda)*(cosU1*sinU2-sinU1*cosU2*cosLambda))

		if sinSigma == 0 {
			return 0 // Coincident points
		}

		cosSigma = sinU1*sinU2 + cosU1*cosU2*cosLambda
		sigma = math.Atan2(sinSigma, cosSigma)
		alpha := math.Asin(cosU1 * cosU2 * sinLambda / sinSigma)
		cosSqAlpha = math.Cos(alpha) * math.Cos(alpha)
		cos2SigmaM = cosSigma - 2*sinU1*sinU2/cosSqAlpha

		C := f / 16 * cosSqAlpha * (4 + f*(4-3*cosSqAlpha))
		lambdaP = lambda
		lambda = L + (1-C)*f*math.Sin(alpha)*
			(sigma+C*sinSigma*(cos2SigmaM+C*cosSigma*(-1+2*cos2SigmaM*cos2SigmaM)))
	}

	if circleCount == 0 {
		return math.NaN() // Formula failed to converge
	}

	uSq := cosSqAlpha * (a*a - b*b) / (b * b)
	A := 1 + uSq/16384*(4096+uSq*(-768+uSq*(320-175*uSq)))
	B := uSq / 1024 * (256 + uSq*(-128+uSq*(74-47*uSq)))
	deltaSigma := B * sinSigma * (cos2SigmaM + B/4*(cosSigma*(-1+2*cos2SigmaM*cos2SigmaM)-
		B/6*cos2SigmaM*(-3+4*sinSigma*sinSigma)*(-3+4*cos2SigmaM*cos2SigmaM)))

	// Result is in meters, convert to kilometers
	result := b * A * (sigma - deltaSigma) / 1000
	return result
}

// toRadians converts degrees to radians
func toRadians(angle float64) float64 {
	return angle * math.Pi / 180
}
