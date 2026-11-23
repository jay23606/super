package aggregate

import (
	"github.com/superdb/super"
	"github.com/superdb/super/runtime/expr"
	"github.com/superdb/super/vector"
	"github.com/superdb/super/vector/vhash"
)

// one aggTable per fixed set of types of aggs and keys.
type aggTable interface {
	update([]vector.Any, []vector.Any)
	materialize() vector.Any
}

type superTable struct {
	aggs        []*expr.Aggregator
	builder     *vector.RecordBuilder
	partialsIn  bool
	partialsOut bool
	table       *vhash.Table
	funcs       []expr.AggFunc // one per aggregation, holding every group
	sctx        *super.Context
}

var _ aggTable = (*superTable)(nil)

func newSuperTable(a *Aggregate, keyTypes []super.Type) *superTable {
	var funcs []expr.AggFunc
	for _, agg := range a.aggs {
		funcs = append(funcs, agg.Pattern())
	}
	return &superTable{
		aggs:        a.aggs,
		builder:     a.builder,
		partialsIn:  a.partialsIn,
		partialsOut: a.partialsOut,
		table:       vhash.NewTable(keyTypes),
		funcs:       funcs,
		sctx:        a.sctx,
	}
}

func (s *superTable) update(keys []vector.Any, args []vector.Any) {
	keyIDs, created := s.table.FindOrInsert(keys)
	if len(created) > 0 {
		for _, f := range s.funcs {
			f.Resize(s.table.Len())
		}
	}
	for i, arg := range args {
		if s.partialsIn {
			s.funcs[i].ConsumeAsPartial(keyIDs, arg)
			continue
		}
		groups := keyIDs
		if s.aggs[i].Where != nil {
			if groups, arg = dropFiltered(groups, arg); arg == nil {
				continue
			}
		}
		s.funcs[i].Consume(groups, arg)
	}
}

// dropFiltered returns groups and vec without the slots dropped by a filter
// clause, which expr.Aggregator fills with plain nones, or a nil vec if every
// slot was dropped.
func dropFiltered(groups []uint32, vec vector.Any) ([]uint32, vector.Any) {
	d, ok := vec.(*vector.Dynamic)
	if !ok {
		if vec.Type() == super.TypeNone {
			return nil, nil
		}
		return groups, vec
	}
	isNone := make([]bool, len(d.Values))
	var rest []vector.Any
	for tag, val := range d.Values {
		isNone[tag] = val.Type() == super.TypeNone
		if !isNone[tag] {
			rest = append(rest, val)
		}
	}
	if len(rest) == 0 {
		return nil, nil
	}
	if len(rest) == len(d.Values) {
		return groups, vec
	}
	kept := make([]uint32, 0, len(d.Tags))
	for slot, tag := range d.Tags {
		if !isNone[tag] {
			kept = append(kept, groups[slot])
		}
	}
	if len(rest) == 1 {
		// The values of the only other tag are the kept slots in order.
		return kept, rest[0]
	}
	index := make([]uint32, 0, len(kept))
	for slot, tag := range d.Tags {
		if !isNone[tag] {
			index = append(index, uint32(slot))
		}
	}
	return kept, vector.Pick(d, index)
}

func (s *superTable) materialize() vector.Any {
	if s.table.Len() == 0 {
		return vector.NewNull(0)
	}
	vecs := s.table.Keys(s.sctx)
	for _, f := range s.funcs {
		if s.partialsOut {
			vecs = append(vecs, f.ResultAsPartial(s.sctx))
		} else {
			vecs = append(vecs, f.Result(s.sctx))
		}
	}
	// Since aggs can return dynamic values need to do apply to create record.
	return vector.Apply(vector.ApplyNone, func(vecs ...vector.Any) vector.Any {
		return s.builder.New(vecs)
	}, vecs...)
}
