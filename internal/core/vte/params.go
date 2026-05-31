package vte

const (
	maxParams    = 32
	maxSubParams = 8
)

// paramsBuf accumulates CSI/DCS parameters without heap allocation.
// Parameters are separated by ';', sub-parameters within a parameter by ':'.
//
// Example: "38:2:255:0:0;1" → [[38,2,255,0,0],[1]]
type paramsBuf struct {
	flat [maxParams * maxSubParams]uint16 // sub-param values, indexed by param*maxSubParams+sub
	nSub [maxParams]int                   // number of sub-params per param (0 = empty/default)
	n    int                              // number of params seen
}

// reset clears all accumulated params. Called on entering Escape/CsiEntry/DcsEntry.
func (b *paramsBuf) reset() {
	b.n = 0
	b.nSub[0] = 0
}

// param processes a single byte from the Param action (0x30-0x3B).
func (b *paramsBuf) param(c byte) {
	switch c {
	case ';':
		if b.n == 0 {
			b.n = 1
			b.nSub[0] = 0
		}
		if b.n < maxParams {
			b.n++
			b.nSub[b.n-1] = 0
		}

	case ':':
		if b.n == 0 {
			b.n = 1
			b.nSub[0] = 1
			b.flat[0] = 0
		} else {
			i := b.n - 1
			if b.nSub[i] == 0 {
				b.nSub[i] = 1
				b.flat[i*maxSubParams] = 0
			}
			if b.nSub[i] < maxSubParams {
				b.nSub[i]++
				b.flat[i*maxSubParams+b.nSub[i]-1] = 0
			}
		}

	default: // digit 0x30-0x39
		if b.n == 0 {
			b.n = 1
			b.nSub[0] = 0
		}
		i := b.n - 1
		if b.nSub[i] == 0 {
			b.nSub[i] = 1
			b.flat[i*maxSubParams] = 0
		}
		j := b.nSub[i] - 1
		idx := i*maxSubParams + j
		v := uint32(b.flat[idx])*10 + uint32(c-'0')
		if v <= 0xFFFF {
			b.flat[idx] = uint16(v)
		}
	}
}

// build fills ps with per-param slices backed by flat, returning updated slices.
// The returned slices share backing storage and are valid only until the next
// call to reset() or build().
func (b *paramsBuf) build(ps [][]uint16, flat []uint16) ([][]uint16, []uint16) {
	ps = ps[:0]
	flat = flat[:0]
	for i := 0; i < b.n; i++ {
		start := len(flat)
		n := b.nSub[i]
		if n == 0 {
			flat = append(flat, 0)
		} else {
			base := i * maxSubParams
			for j := 0; j < n; j++ {
				flat = append(flat, b.flat[base+j])
			}
		}
		ps = append(ps, flat[start:])
	}
	return ps, flat
}
