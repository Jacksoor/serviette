package pricing

import (
	"time"
)

type Usage struct {
	CPUTime time.Duration
	Memory  int64
}

type Pricer interface {
	MaxUsage(cost int64) *Usage
	Cost(usage *Usage) int64
}

type FactorPricer struct {
	CPUTimeNum int64
	CPUTimeDen int64

	MemoryNum int64
	MemoryDen int64
}

func (p *FactorPricer) MaxUsage(cost int64) *Usage {
	return &Usage{
		CPUTime: time.Duration(cost*p.CPUTimeDen/p.CPUTimeNum) * time.Millisecond,
		Memory:  cost * p.MemoryDen / p.MemoryNum,
	}
}

func (p *FactorPricer) Cost(usage *Usage) int64 {
	return int64(usage.CPUTime/time.Millisecond)*p.CPUTimeNum/p.CPUTimeDen + usage.Memory*p.MemoryNum/p.MemoryDen
}
