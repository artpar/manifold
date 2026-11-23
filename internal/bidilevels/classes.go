package bidilevels

import "golang.org/x/text/unicode/bidi"

type Class uint

const (
	L            Class = Class(bidi.L)
	R            Class = Class(bidi.R)
	EN           Class = Class(bidi.EN)
	ES           Class = Class(bidi.ES)
	ET           Class = Class(bidi.ET)
	AN           Class = Class(bidi.AN)
	CS           Class = Class(bidi.CS)
	B            Class = Class(bidi.B)
	S            Class = Class(bidi.S)
	WS           Class = Class(bidi.WS)
	ON           Class = Class(bidi.ON)
	BN           Class = Class(bidi.BN)
	NSM          Class = Class(bidi.NSM)
	AL           Class = Class(bidi.AL)
	Control      Class = Class(bidi.Control)
	LRO          Class = Class(bidi.LRO)
	RLO          Class = Class(bidi.RLO)
	LRE          Class = Class(bidi.LRE)
	RLE          Class = Class(bidi.RLE)
	PDF          Class = Class(bidi.PDF)
	LRI          Class = Class(bidi.LRI)
	RLI          Class = Class(bidi.RLI)
	FSI          Class = Class(bidi.FSI)
	PDI          Class = Class(bidi.PDI)
	unknownClass       = ^Class(0)
)
