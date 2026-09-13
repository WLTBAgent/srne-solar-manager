package controller

import "sort"

// Limits for merged Modbus reads. Although FC03 allows 125 registers
// per request, the gateways inside real SRNE integrated units time out
// or answer with "gateway path unavailable" well before that (reads of
// 37+ registers were observed failing while ~20 succeeded), so the
// planner stays conservative.
const (
	MaxBlockCount = 24 // max registers per Modbus read
	MaxBlockGap   = 2  // max unused addresses tolerated inside a block
)

// Block is a planned contiguous Modbus read covering one or more registers.
type Block struct {
	Start uint16
	Count uint16
	Regs  []*Register // registers covered by this block, in address order
}

// PlanRegisterBlocks merges the given registers into as few contiguous
// block reads as possible. A register extends a block when its start
// address lies within maxGap registers of the block's current end, and
// the merged read stays within maxCount registers.
func PlanRegisterBlocks(regs []*Register, maxCount, maxGap int) []Block {
	sorted := make([]*Register, len(regs))
	copy(sorted, regs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Address < sorted[j].Address })

	var blocks []Block
	var cur *Block

	for _, reg := range sorted {
		start := int(reg.Address)
		end := start + reg.Length // exclusive
		if cur != nil {
			curEnd := int(cur.Start) + int(cur.Count)
			if start-int(curEnd) <= maxGap && end-int(cur.Start) <= maxCount {
				if end > curEnd {
					cur.Count = uint16(end - int(cur.Start))
				}
				cur.Regs = append(cur.Regs, reg)
				continue
			}
		}
		blocks = append(blocks, Block{Start: reg.Address, Count: uint16(reg.Length), Regs: []*Register{reg}})
		cur = &blocks[len(blocks)-1]
	}
	return blocks
}
