// Package vte implements the Paul Williams VT500 ANSI terminal escape sequence
// parser (https://vt100.net/emu/dec_ansi_parser), ported from alacritty/vte
// (Rust) via go-vte (Go), following the same strategy as Rio's copa crate.
//
// The parser is a table-driven state machine. Each (state, byte) pair maps to
// an (action, nextState) transition packed into a single uint8. Anywhere-
// transitions (0x18, 0x1A, 0x1B, 0x80-0x9F) override state-local entries and
// are applied last in the table build so they always win.
//
// UTF-8 multi-byte sequences are intercepted before the state machine: once a
// lead byte (≥0xC0) is seen in Ground state, continuation bytes are consumed
// directly without going through the table.
package vte

import (
	"unicode"
	"unicode/utf8"
)

// --- State and action encoding ---

type state uint8
type action uint8

// States — 14 values, fit in low nibble of uint8.
const (
	stateGround    state = 0
	stateEscape    state = 1
	stateEscInter  state = 2 // EscapeIntermediate
	stateCsiEntry  state = 3
	stateCsiParam  state = 4
	stateCsiInter  state = 5 // CsiIntermediate
	stateCsiIgnore state = 6
	stateDcsEntry  state = 7
	stateDcsParam  state = 8
	stateDcsInter  state = 9  // DcsIntermediate
	stateDcsPass   state = 10 // DcsPassthrough
	stateDcsIgnore state = 11
	stateOscString state = 12
	stateSosPmApc  state = 13 // SosPmApcString
	numStates            = 14
)

// Actions — 16 values, fit in high nibble of uint8.
const (
	actionNone        action = 0
	actionPrint       action = 1
	actionExecute     action = 2
	actionIgnore      action = 3
	actionCollect     action = 4
	actionParam       action = 5
	actionEscDispatch action = 6
	actionCsiDispatch action = 7
	actionHook        action = 8
	actionPut         action = 9
	actionUnhook      action = 10 // unused in table; fires on DcsPass exit
	actionOscPut      action = 11
	actionOscEnd      action = 12 // 0x07 BEL inside OscString
	actionBeginUtf8   action = 13
	// 14, 15 reserved
)

// table[state][byte] = (action << 4) | nextState
var table [numStates][256]uint8

func pack(a action, s state) uint8 { return uint8(a)<<4 | uint8(s) }
func unpack(v uint8) (action, state) {
	return action(v >> 4), state(v & 0x0F)
}

// fillTable fills a byte range in the given state with the same (action, nextState).
func fillTable(st state, lo, hi int, a action, next state) {
	for b := lo; b <= hi; b++ {
		table[st][b] = pack(a, next)
	}
}

// fillC0Table fills C0 control byte ranges (0x00-0x17, 0x19, 0x1C-0x1F).
// 0x18, 0x1A, 0x1B are excluded — the "anywhere" transitions override them.
func fillC0Table(st state, a action, next state) {
	fillTable(st, 0x00, 0x17, a, next)
	table[st][0x19] = pack(a, next)
	fillTable(st, 0x1C, 0x1F, a, next)
}

func initGroundTable() {
	fillC0Table(stateGround, actionExecute, stateGround)
	fillTable(stateGround, 0x20, 0x7E, actionPrint, stateGround)
	table[stateGround][0x7F] = pack(actionIgnore, stateGround)
	fillTable(stateGround, 0xC0, 0xFF, actionBeginUtf8, stateGround)
}

func initEscapeTable() {
	fillC0Table(stateEscape, actionExecute, stateEscape)
	fillTable(stateEscape, 0x20, 0x2F, actionCollect, stateEscInter)
	fillTable(stateEscape, 0x30, 0x4F, actionEscDispatch, stateGround)
	table[stateEscape][0x50] = pack(actionNone, stateDcsEntry)
	fillTable(stateEscape, 0x51, 0x57, actionEscDispatch, stateGround)
	table[stateEscape][0x58] = pack(actionNone, stateSosPmApc)
	table[stateEscape][0x59] = pack(actionEscDispatch, stateGround)
	table[stateEscape][0x5A] = pack(actionEscDispatch, stateGround)
	table[stateEscape][0x5B] = pack(actionNone, stateCsiEntry)
	table[stateEscape][0x5C] = pack(actionEscDispatch, stateGround)
	table[stateEscape][0x5D] = pack(actionNone, stateOscString)
	table[stateEscape][0x5E] = pack(actionNone, stateSosPmApc)
	table[stateEscape][0x5F] = pack(actionNone, stateSosPmApc)
	fillTable(stateEscape, 0x60, 0x7E, actionEscDispatch, stateGround)
	table[stateEscape][0x7F] = pack(actionIgnore, stateEscape)
}

func initEscInterTable() {
	fillC0Table(stateEscInter, actionExecute, stateEscInter)
	fillTable(stateEscInter, 0x20, 0x2F, actionCollect, stateEscInter)
	fillTable(stateEscInter, 0x30, 0x7E, actionEscDispatch, stateGround)
	table[stateEscInter][0x7F] = pack(actionIgnore, stateEscInter)
}

func initCsiEntryTable() {
	fillC0Table(stateCsiEntry, actionExecute, stateCsiEntry)
	fillTable(stateCsiEntry, 0x20, 0x2F, actionCollect, stateCsiInter)
	fillTable(stateCsiEntry, 0x30, 0x3B, actionParam, stateCsiParam)   // digits + ';'
	table[stateCsiEntry][0x3A] = pack(actionParam, stateCsiParam)      // ':' sub-param separator
	fillTable(stateCsiEntry, 0x3C, 0x3F, actionCollect, stateCsiParam) // private markers <=>?
	fillTable(stateCsiEntry, 0x40, 0x7E, actionCsiDispatch, stateGround)
	table[stateCsiEntry][0x7F] = pack(actionIgnore, stateCsiEntry)
}

func initCsiParamTable() {
	fillC0Table(stateCsiParam, actionExecute, stateCsiParam)
	fillTable(stateCsiParam, 0x20, 0x2F, actionCollect, stateCsiInter)
	fillTable(stateCsiParam, 0x30, 0x3B, actionParam, stateCsiParam)
	table[stateCsiParam][0x3A] = pack(actionParam, stateCsiParam)
	fillTable(stateCsiParam, 0x3C, 0x3F, actionNone, stateCsiIgnore) // conflicting private → ignore
	fillTable(stateCsiParam, 0x40, 0x7E, actionCsiDispatch, stateGround)
	table[stateCsiParam][0x7F] = pack(actionIgnore, stateCsiParam)
}

func initCsiInterTable() {
	fillC0Table(stateCsiInter, actionExecute, stateCsiInter)
	fillTable(stateCsiInter, 0x20, 0x2F, actionCollect, stateCsiInter)
	fillTable(stateCsiInter, 0x30, 0x3F, actionNone, stateCsiIgnore) // param after intermediate → ignore
	fillTable(stateCsiInter, 0x40, 0x7E, actionCsiDispatch, stateGround)
	table[stateCsiInter][0x7F] = pack(actionIgnore, stateCsiInter)
}

func initCsiIgnoreTable() {
	fillC0Table(stateCsiIgnore, actionExecute, stateCsiIgnore)
	fillTable(stateCsiIgnore, 0x20, 0x3F, actionIgnore, stateCsiIgnore)
	fillTable(stateCsiIgnore, 0x40, 0x7E, actionNone, stateGround) // discard — do NOT dispatch
	table[stateCsiIgnore][0x7F] = pack(actionIgnore, stateCsiIgnore)
}

func initDcsTables() {
	// DcsEntry
	fillC0Table(stateDcsEntry, actionIgnore, stateDcsEntry)
	fillTable(stateDcsEntry, 0x20, 0x2F, actionCollect, stateDcsInter)
	fillTable(stateDcsEntry, 0x30, 0x3B, actionParam, stateDcsParam)
	table[stateDcsEntry][0x3A] = pack(actionParam, stateDcsParam)
	fillTable(stateDcsEntry, 0x3C, 0x3F, actionCollect, stateDcsParam)
	fillTable(stateDcsEntry, 0x40, 0x7E, actionHook, stateDcsPass)
	table[stateDcsEntry][0x7F] = pack(actionIgnore, stateDcsEntry)
	// DcsParam
	fillC0Table(stateDcsParam, actionIgnore, stateDcsParam)
	fillTable(stateDcsParam, 0x20, 0x2F, actionCollect, stateDcsInter)
	fillTable(stateDcsParam, 0x30, 0x3B, actionParam, stateDcsParam)
	table[stateDcsParam][0x3A] = pack(actionParam, stateDcsParam)
	fillTable(stateDcsParam, 0x3C, 0x3F, actionNone, stateDcsIgnore)
	fillTable(stateDcsParam, 0x40, 0x7E, actionHook, stateDcsPass)
	table[stateDcsParam][0x7F] = pack(actionIgnore, stateDcsParam)
	// DcsIntermediate
	fillC0Table(stateDcsInter, actionIgnore, stateDcsInter)
	fillTable(stateDcsInter, 0x20, 0x2F, actionCollect, stateDcsInter)
	fillTable(stateDcsInter, 0x30, 0x3F, actionNone, stateDcsIgnore)
	fillTable(stateDcsInter, 0x40, 0x7E, actionHook, stateDcsPass)
	table[stateDcsInter][0x7F] = pack(actionIgnore, stateDcsInter)
	// DcsPassthrough: entry=Hook (actionHook on enter), exit=Unhook (exitState).
	fillC0Table(stateDcsPass, actionPut, stateDcsPass)
	fillTable(stateDcsPass, 0x20, 0x7E, actionPut, stateDcsPass)
	table[stateDcsPass][0x7F] = pack(actionIgnore, stateDcsPass)
	// DcsIgnore
	fillC0Table(stateDcsIgnore, actionIgnore, stateDcsIgnore)
	fillTable(stateDcsIgnore, 0x20, 0x7F, actionIgnore, stateDcsIgnore)
}

func initOscSosTables() {
	// OscString: 0x07 (BEL) is explicit terminator; entry clears oscRaw in enterState.
	table[stateOscString][0x07] = pack(actionOscEnd, stateGround)
	fillTable(stateOscString, 0x20, 0xFF, actionOscPut, stateOscString)
	// 0x80-0x9F will be overridden by the "anywhere" loop.
	// SosPmApcString
	fillC0Table(stateSosPmApc, actionIgnore, stateSosPmApc)
	fillTable(stateSosPmApc, 0x20, 0x7F, actionIgnore, stateSosPmApc)
}

func initAnywhereTable() {
	for st := state(0); st < numStates; st++ {
		table[st][0x18] = pack(actionExecute, stateGround)
		table[st][0x1A] = pack(actionExecute, stateGround)
		table[st][0x1B] = pack(actionNone, stateEscape)
		for b := 0x80; b <= 0x8F; b++ {
			table[st][b] = pack(actionExecute, stateGround)
		}
		table[st][0x90] = pack(actionNone, stateDcsEntry)
		for b := 0x91; b <= 0x97; b++ {
			table[st][b] = pack(actionExecute, stateGround)
		}
		table[st][0x98] = pack(actionNone, stateSosPmApc)
		table[st][0x99] = pack(actionExecute, stateGround)
		table[st][0x9A] = pack(actionExecute, stateGround)
		table[st][0x9B] = pack(actionNone, stateCsiEntry)
		table[st][0x9C] = pack(actionNone, stateGround) // ST — exits DcsPass/OscString via exitState
		table[st][0x9D] = pack(actionNone, stateOscString)
		table[st][0x9E] = pack(actionNone, stateSosPmApc)
		table[st][0x9F] = pack(actionNone, stateSosPmApc)
	}
}

func init() {
	for st := state(0); st < numStates; st++ {
		for b := 0; b < 256; b++ {
			table[st][b] = pack(actionIgnore, st)
		}
	}
	initGroundTable()
	initEscapeTable()
	initEscInterTable()
	initCsiEntryTable()
	initCsiParamTable()
	initCsiInterTable()
	initCsiIgnoreTable()
	initDcsTables()
	initOscSosTables()
	initAnywhereTable()
}

// --- Parser ---

// Parser implements the VT500 state machine.
// It is not safe for concurrent use; each terminal session needs its own instance.
type Parser struct {
	state state

	// Collected intermediate bytes (0x20-0x2F) before the final byte.
	intermediates [2]byte
	nInter        int

	// Parameter accumulator.
	params paramsBuf

	// OSC raw byte accumulation.
	oscRaw []byte

	// UTF-8 multi-byte sequence accumulation.
	utf8Seq  [4]byte
	utf8Len  int
	utf8Need int // remaining continuation bytes expected

	// Scratch slices reused across dispatches (valid only during the Performer callback).
	paramSlice [][]uint16
	flatSlice  []uint16
}

// New returns a Parser in the Ground state.
func New() *Parser {
	return &Parser{
		state:      stateGround,
		oscRaw:     make([]byte, 0, 256),
		paramSlice: make([][]uint16, 0, maxParams),
		flatSlice:  make([]uint16, 0, maxParams*4),
	}
}

// Advance processes a chunk of bytes, calling perf for each decoded sequence.
// This is the hot path: zero heap allocations per call in the common case.
func (p *Parser) Advance(perf Performer, data []byte) {
	for i := 0; i < len(data); i++ {
		p.advance(perf, data[i])
	}
}

// utf8Cont processes a UTF-8 continuation byte. Returns true if b was consumed.
// When false is returned, the caller should process b through the state machine.
func (p *Parser) utf8Cont(perf Performer, b byte) bool {
	if b < 0x80 || b > 0xBF {
		// Unexpected byte: emit replacement and let caller process b normally.
		perf.Print(unicode.ReplacementChar)
		p.utf8Need = 0
		p.utf8Len = 0
		return false
	}
	p.utf8Seq[p.utf8Len] = b
	p.utf8Len++
	p.utf8Need--
	if p.utf8Need == 0 {
		r, _ := utf8.DecodeRune(p.utf8Seq[:p.utf8Len])
		if r == utf8.RuneError {
			r = unicode.ReplacementChar
		}
		perf.Print(r)
		p.utf8Len = 0
	}
	return true
}

func (p *Parser) advance(perf Performer, b byte) {
	// UTF-8 multi-byte continuation — handled before the state machine.
	if p.utf8Need > 0 && p.utf8Cont(perf, b) {
		return
	}

	act, next := unpack(table[p.state][b])

	if next != p.state {
		p.exitState(perf, p.state, act)
	}

	p.doAction(perf, act, b)

	if next != p.state {
		p.enterState(next)
		p.state = next
	}
}

// exitState fires the exit action for states that have one.
// act is the action that triggered the state transition; it is used to avoid
// double-dispatching when the action itself handles the dispatch (e.g. OscEnd).
func (p *Parser) exitState(perf Performer, st state, act action) {
	switch st {
	case stateDcsPass:
		perf.DCSUnhook()
	case stateOscString:
		// 0x07 (BEL) fires actionOscEnd which dispatches; all other exits dispatch here.
		if act != actionOscEnd {
			p.dispatchOSC(perf, false)
		}
	}
}

// enterState fires the entry action for states that have one.
func (p *Parser) enterState(st state) {
	switch st {
	case stateEscape, stateCsiEntry, stateDcsEntry:
		p.params.reset()
		p.nInter = 0
	case stateOscString:
		p.oscRaw = p.oscRaw[:0]
	}
}

// doAction executes the given action for byte b.
func (p *Parser) doAction(perf Performer, act action, b byte) {
	switch act {
	case actionNone, actionIgnore:
		// nothing

	case actionPrint:
		perf.Print(rune(b))

	case actionBeginUtf8:
		switch {
		case b < 0xE0: // 2-byte sequence
			p.utf8Seq[0] = b
			p.utf8Len = 1
			p.utf8Need = 1
		case b < 0xF0: // 3-byte sequence
			p.utf8Seq[0] = b
			p.utf8Len = 1
			p.utf8Need = 2
		default: // 4-byte sequence
			p.utf8Seq[0] = b
			p.utf8Len = 1
			p.utf8Need = 3
		}

	case actionExecute:
		perf.Execute(b)

	case actionCollect:
		if p.nInter < len(p.intermediates) {
			p.intermediates[p.nInter] = b
			p.nInter++
		}
		// Silently drop if more than 2 intermediates (rare, malformed sequence).

	case actionParam:
		p.params.param(b)

	case actionEscDispatch:
		perf.EscDispatch(p.intermediates[:p.nInter], b)

	case actionCsiDispatch:
		ps, flat := p.params.build(p.paramSlice, p.flatSlice)
		p.paramSlice = ps
		p.flatSlice = flat
		perf.CSI(ps, p.intermediates[:p.nInter], false, rune(b))

	case actionHook:
		ps, flat := p.params.build(p.paramSlice, p.flatSlice)
		p.paramSlice = ps
		p.flatSlice = flat
		perf.DCS(ps, p.intermediates[:p.nInter], false, rune(b))

	case actionPut:
		perf.DCSPut(b)

	case actionOscPut:
		p.oscRaw = append(p.oscRaw, b)

	case actionOscEnd:
		p.dispatchOSC(perf, true)
	}
}

// dispatchOSC splits the accumulated OSC bytes by ';' and calls perf.OSC.
func (p *Parser) dispatchOSC(perf Performer, bellTerminated bool) {
	raw := p.oscRaw
	// Split by ';' into at most maxParams+1 fields.
	var params [maxParams + 1][]byte
	n := 0
	start := 0
	for i, c := range raw {
		if c == ';' && n < maxParams {
			params[n] = raw[start:i]
			n++
			start = i + 1
		}
	}
	params[n] = raw[start:]
	n++
	perf.OSC(params[:n], bellTerminated)
	p.oscRaw = p.oscRaw[:0]
}
