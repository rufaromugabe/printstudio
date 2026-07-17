//go:build clipper2 && cgo

package production

import (
	"fmt"
	"math"

	clipper "github.com/epit3d/goclipper2/goclipper2"
)

const clipperScale = 1000.0 // one integer unit is one micron

func Clipper2Available() bool { return true }
func BooleanPolygons(subject, clips PolygonPaths, operation BooleanOperation) (PolygonPaths, error) {
	s, err := toClipper(subject)
	if err != nil {
		return nil, err
	}
	defer s.Delete()
	c, err := toClipper(clips)
	if err != nil {
		return nil, err
	}
	defer c.Delete()
	var result *clipper.Paths64
	switch operation {
	case BooleanUnion:
		result = s.Union(*c, clipper.EvenOdd)
	case BooleanDifference:
		result = s.Difference(*c, clipper.EvenOdd)
	case BooleanIntersection:
		result = s.Intersect(*c, clipper.EvenOdd)
	case BooleanXOR:
		result = s.Xor(*c, clipper.EvenOdd)
	default:
		return nil, fmt.Errorf("unsupported Boolean operation %q", operation)
	}
	defer result.Delete()
	return fromClipper(result), nil
}
func OffsetPolygons(paths PolygonPaths, deltaMM float64, join OffsetJoin, miterLimit float64) (PolygonPaths, error) {
	input, err := toClipper(paths)
	if err != nil {
		return nil, err
	}
	defer input.Delete()
	jt := clipper.RoundJoin
	switch join {
	case JoinRound, "":
	case JoinSquare:
		jt = clipper.SquareJoin
	case JoinMiter:
		jt = clipper.MiterJoin
	default:
		return nil, fmt.Errorf("unsupported offset join %q", join)
	}
	if miterLimit <= 0 {
		miterLimit = 2
	}
	result := input.Inflate(deltaMM*clipperScale, jt, clipper.PolygonEnd, miterLimit)
	defer result.Delete()
	return fromClipper(result), nil
}
func toClipper(paths PolygonPaths) (*clipper.Paths64, error) {
	result := clipper.NewPaths64()
	for i, ring := range paths {
		if len(ring) < 3 {
			result.Delete()
			return nil, fmt.Errorf("path %d has fewer than three points", i)
		}
		path := clipper.NewPath64()
		for _, point := range ring {
			if math.IsNaN(point.X) || math.IsNaN(point.Y) || math.IsInf(point.X, 0) || math.IsInf(point.Y, 0) || math.Abs(point.X) > 1e9 || math.Abs(point.Y) > 1e9 {
				path.Delete()
				result.Delete()
				return nil, fmt.Errorf("path %d contains an invalid coordinate", i)
			}
			path.AddPoint(*clipper.NewPoint64(int64(math.Round(point.X*clipperScale)), int64(math.Round(point.Y*clipperScale))))
		}
		result.AddPath(*path)
		path.Delete()
	}
	return result, nil
}
func fromClipper(paths *clipper.Paths64) PolygonPaths {
	result := make(PolygonPaths, 0, paths.Length())
	for i := int64(0); i < paths.Length(); i++ {
		ring := make([]PolygonPoint, 0, paths.PathLength(i))
		for j := int64(0); j < paths.PathLength(i); j++ {
			point := paths.GetPoint(i, j)
			ring = append(ring, PolygonPoint{X: float64(point.X()) / clipperScale, Y: float64(point.Y()) / clipperScale})
		}
		result = append(result, ring)
	}
	return result
}
