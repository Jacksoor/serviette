package pricing

import (
	"time"
)

type Usage struct {
	RealTime time.Duration
	Memory   int64
}

type Pricer interface {
	MaxUsage(cost int64) *Usage
	Cost(usage *Usage) int64
}

type FactorPricer struct {
	RealTimeNum time.Duration
	RealTimeDen time.Duration

	MemoryNum int64
	MemoryDen int64
}

func (p *FactorPricer) MaxUsage(cost int64) *Usage {
	return &Usage{
		RealTime: time.Duration(cost) * p.RealTimeDen / p.RealTimeNum,
		Memory:   cost * p.MemoryDen / p.MemoryNum,
	}
}

func (p *FactorPricer) Cost(usage *Usage) int64 {
	return int64(usage.RealTime/time.Millisecond*p.RealTimeNum/p.RealTimeDen) + usage.Memory*p.MemoryNum/p.MemoryDen
}
