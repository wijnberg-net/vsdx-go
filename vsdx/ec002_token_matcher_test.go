package vsdx

import (
	"testing"
)

// a regression case: scaleNonInhCell used to skip cells whose formula CONTAINED the
// substring "Width" or "Height". This false-positived on user-defined cells
// whose names happen to embed those tokens. The fix uses a word-boundary
// regex so only true Width / Height token references are honoured.

func TestEC002_TokenMatcher_PositiveMatches(t *testing.T) {
	positives := []string{
		"Width",
		"Width*0.5",
		"Height",
		"Height/2",
		"Sheet.5!Width",
		"GUARD(Width)",
		"GUARD((BeginX+EndX)*0.5 + Height)",
		"  Width  ",
	}
	for _, f := range positives {
		if !widthHeightTokenRE.MatchString(f) {
			t.Errorf("expected match for %q", f)
		}
	}
}

func TestEC002_TokenMatcher_NegativeMatches(t *testing.T) {
	// These contain "Width" or "Height" as substrings but NOT as bare
	// tokens. Pre-fix they were wrongly skipped by scaleNonInhCell.
	negatives := []string{
		"WidthForegndColor",
		"WidthValue",
		"MyWidth",
		"ScaleHeightFactor",
		"HeightModifier",
		"SubWidget",
		"Showroom",
		"SomeWidthRef + 1",
		"width", // lowercase: not the Visio cell
		"HEIGHT", // uppercase: not the Visio cell
	}
	for _, f := range negatives {
		if widthHeightTokenRE.MatchString(f) {
			t.Errorf("expected NO match for %q (false-positive risk)", f)
		}
	}
}
