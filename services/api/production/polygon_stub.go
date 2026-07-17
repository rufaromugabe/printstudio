//go:build !clipper2 || !cgo

package production

func Clipper2Available() bool { return false }
func BooleanPolygons(subject, clip PolygonPaths, operation BooleanOperation) (PolygonPaths, error) {
	return nil, ErrClipper2Unavailable
}
func OffsetPolygons(paths PolygonPaths, deltaMM float64, join OffsetJoin, miterLimit float64) (PolygonPaths, error) {
	return nil, ErrClipper2Unavailable
}
