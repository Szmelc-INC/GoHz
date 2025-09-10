package main

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
)

type segment struct{ start, end float64 }

// splitBySilence splits input file into segments based on silence spans longer
// than minSilDur seconds. It trims `trim` seconds from the start and end of
// each segment. Returned slice contains paths of created files.
func splitBySilence(cfg *Config, in string, a *Analysis, minSilDur, trim float64) ([]string, error) {
	spans := a.Silence
	dur := a.Probe.Duration
	// build segments between silence spans
	var segs []segment
	start := 0.0
	for _, s := range spans {
		if (s.End - s.Start) >= minSilDur {
			segs = append(segs, segment{start, s.Start})
			start = s.End
		}
	}
	if start < dur {
		segs = append(segs, segment{start, dur})
	}
	if len(segs) <= 1 { // nothing to split
		return nil, nil
	}
	base := strings.TrimSuffix(in, filepath.Ext(in))
	ext := filepath.Ext(in)
	var outs []string
	for i, sg := range segs {
		s := sg.start
		e := sg.end
		if trim > 0 {
			s = math.Min(e, s+trim)
			e = math.Max(s, e-trim)
		}
		out := fmt.Sprintf("%s-part%02d%s", base, i+1, ext)
		args := []string{"-y", "-i", in, "-ss", fmt.Sprintf("%f", s), "-to", fmt.Sprintf("%f", e), "-c", "copy", out}
		if _, err := runCmd(cfg.FFmpegBin, args...); err != nil {
			return outs, fmt.Errorf("ffmpeg split: %w", err)
		}
		fmt.Printf("[+] wrote %s\n", out)
		outs = append(outs, out)
	}
	return outs, nil
}
