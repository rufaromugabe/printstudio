package production

import "errors"

type PolygonPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type PolygonPaths [][]PolygonPoint
type BooleanOperation string
type OffsetJoin string

const (
	BooleanUnion        BooleanOperation = "union"
	BooleanDifference   BooleanOperation = "difference"
	BooleanIntersection BooleanOperation = "intersection"
	BooleanXOR          BooleanOperation = "xor"
	JoinRound           OffsetJoin       = "round"
	JoinSquare          OffsetJoin       = "square"
	JoinMiter           OffsetJoin       = "miter"
)

var ErrClipper2Unavailable = errors.New("Clipper2 native backend is unavailable; build with -tags clipper2 and CGO_ENABLED=1")
