package tree

import (
	"encoding/binary"
	"reflect"
	"unsafe"

	"github.com/cespare/xxhash/v2"
)

type ProfileReader struct {
	sampleTypesFilter func(string) bool
	cache             labelsCache
}

func NewProfileReader() *ProfileReader { return new(ProfileReader) }

func (r *ProfileReader) SampleTypeFilter(fn func(string) bool) *ProfileReader {
	r.sampleTypesFilter = fn
	return r
}

func (r *ProfileReader) Reset() {
	r.cache = make(labelsCache)
}

func (r *ProfileReader) Load(st int, labels Labels) (*Tree, bool) {
	e, ok := r.cache.get(st, labels.Hash())
	if !ok {
		return nil, false
	}
	return e.Tree, true
}

func (r *ProfileReader) Read(x *Profile, fn func(vt *ValueType, l Labels, t *Tree) (keep bool, err error)) error {
	sampleTypes := make([]int, 0, len(x.SampleType))
	for i, s := range x.SampleType {
		if r.sampleTypesFilter != nil && !r.sampleTypesFilter(x.StringTable[s.Type]) {
			continue
		}
		sampleTypes = append(sampleTypes, i)
	}
	if len(sampleTypes) == 0 {
		return nil
	}
	c := make(labelsCache)
	readTrees(x, sampleTypes, c, NewFinder(x))
	for sampleType, entries := range c {
		for h, e := range entries {
			keep, err := fn(x.SampleType[sampleType], e.Labels, e.Tree)
			if err != nil {
				return err
			}
			if !keep {
				c.remove(sampleType, h)
			}
		}
	}
	r.cache = c
	return nil
}

func (x *Profile) ResolveLabels(l Labels) map[string]string {
	m := make(map[string]string)
	for _, label := range l {
		if label.Str != 0 {
			m[x.StringTable[label.Key]] = x.StringTable[label.Str]
		}
	}
	return m
}

type Labels []*Label

func (l Labels) Hash() uint64 {
	h := xxhash.New()
	t := make([]byte, 16)
	for _, x := range l {
		if x.Str == 0 {
			continue
		}
		binary.LittleEndian.PutUint64(t[0:8], uint64(x.Key))
		binary.LittleEndian.PutUint64(t[8:16], uint64(x.Str))
		_, _ = h.Write(t)
	}
	return h.Sum64()
}

func readTrees(x *Profile, sampleTypes []int, c labelsCache, f Finder) {
	stack := make([][]byte, 0, 16)
	for _, s := range x.Sample {
		for i := len(s.LocationId) - 1; i >= 0; i-- {
			// Resolve stack.
			loc, ok := f.FindLocation(s.LocationId[i])
			if !ok {
				continue
			}
			// Multiple line indicates this location has inlined functions,
			// where the last entry represents the caller into which the
			// preceding entries were inlined.
			//
			// E.g., if memcpy() is inlined into printf:
			//    line[0].function_name == "memcpy"
			//    line[1].function_name == "printf"
			//
			// Therefore iteration goes in reverse order.
			for j := len(loc.Line) - 1; j >= 0; j-- {
				fn, ok := f.FindFunction(loc.Line[j].FunctionId)
				if !ok {
					continue
				}
				stack = append(stack, unsafeStrToSlice(x.StringTable[fn.Name]))
			}
		}
		// Insert tree nodes.
		for _, sampleType := range sampleTypes {
			if v := uint64(s.Value[sampleType]); v != 0 {
				c.getOrCreate(sampleType, s.Label).InsertStack(stack, v)
			}
		}
		stack = stack[:0]
	}
}

func unsafeStrToSlice(s string) []byte {
	return (*[0x7fff0000]byte)(unsafe.Pointer((*reflect.StringHeader)(unsafe.Pointer(&s)).Data))[:len(s):len(s)]
}

// sample type (index) -> labels hash -> entry
type labelsCache map[int]map[uint64]*labelsCacheEntry

type labelsCacheEntry struct {
	Labels
	*Tree
}

func newCacheEntry(l Labels) *labelsCacheEntry {
	return &labelsCacheEntry{Tree: New(), Labels: l}
}

func (c labelsCache) getOrCreate(sampleType int, l Labels) *labelsCacheEntry {
	p, ok := c[sampleType]
	if !ok {
		e := newCacheEntry(l)
		c[sampleType] = map[uint64]*labelsCacheEntry{l.Hash(): e}
		return e
	}
	h := l.Hash()
	e, found := p[h]
	if !found {
		e = newCacheEntry(l)
		p[h] = e
	}
	return e
}

func (c labelsCache) get(sampleType int, h uint64) (*labelsCacheEntry, bool) {
	p, ok := c[sampleType]
	if !ok {
		return nil, false
	}
	x, ok := p[h]
	return x, ok
}

func (c labelsCache) put(sampleType int, e *labelsCacheEntry) {
	p, ok := c[sampleType]
	if !ok {
		p = make(map[uint64]*labelsCacheEntry)
		c[sampleType] = p
	}
	p[e.Hash()] = e
}

func (c labelsCache) remove(sampleType int, h uint64) {
	p, ok := c[sampleType]
	if !ok {
		return
	}
	delete(p, h)
}
