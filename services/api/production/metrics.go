package production

import "sync/atomic"

// Metrics tracks production-pipeline counters for readiness and ops alerts.
type Metrics struct {
	SceneRenders   atomic.Int64
	Packs          atomic.Int64
	Approvals      atomic.Int64
	Failures       atomic.Int64
	ICCTransforms  atomic.Int64
	GangRenders    atomic.Int64
	VectorOps      atomic.Int64
}

func (m *Metrics) Snapshot() map[string]int64 {
	return map[string]int64{
		"sceneRenders":  m.SceneRenders.Load(),
		"packs":         m.Packs.Load(),
		"approvals":     m.Approvals.Load(),
		"failures":      m.Failures.Load(),
		"iccTransforms": m.ICCTransforms.Load(),
		"gangRenders":   m.GangRenders.Load(),
		"vectorOps":     m.VectorOps.Load(),
	}
}

var DefaultMetrics Metrics
