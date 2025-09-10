package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

func writeReport(cfg *Config, a *Analysis, path string) error {
	var s string
	switch strings.ToLower(cfg.Report) {
	case "json":
		buf, _ := json.MarshalIndent(a, "", "  ")
		s = string(buf) + "\n"
	case "md":
		s = renderMD(a)
	default:
		s = renderTXT(a)
	}
	return os.WriteFile(path, []byte(s), 0644)
}

func renderTXT(a *Analysis) string {
	var b strings.Builder
	fmt.Fprintf(&b, "File: %s\nWhen: %s\n\n", a.File, a.When)
	fmt.Fprintf(&b, "Format: %s | Duration: %.3fs | SR: %d Hz | Ch: %d | Bitrate: %d bps | BitDepth: %d\n",
		a.Probe.FormatName, a.Probe.Duration, a.Probe.SampleRate, a.Probe.Channels, a.Probe.BitRate, a.Probe.BitDepth)
	fmt.Fprintf(&b, "Levels: Peak %.2f dBFS | RMS %.2f dBFS | Crest %.2f dB | Headroom %.2f dB",
		a.Level.PeakDB, a.Level.RMSDB, a.Level.CrestDB, a.Level.HeadroomDB)
	if a.Level.TruePeakDBTP != nil {
		fmt.Fprintf(&b, " | TruePeak %.2f dBTP", *a.Level.TruePeakDBTP)
	}
	if a.Level.ClipSamples != nil && a.Level.ClipPercent != nil {
		fmt.Fprintf(&b, " | Clips %d (%.3f%%)", *a.Level.ClipSamples, *a.Level.ClipPercent)
	}
	fmt.Fprintf(&b, " | DC %.4f | ZeroX %.2f | NoiseFloor %.2f dBFS\n",
		a.Level.DCOffset, a.Level.ZeroXRate, a.Level.NoiseFloor)
	if a.Loudness != nil {
		fmt.Fprintf(&b, "LUFS: Integrated %.2f LUFS | Range %.2f LU", a.Loudness.Integrated, a.Loudness.Range)
		if a.Loudness.TruePeak != nil {
			fmt.Fprintf(&b, " | TruePeak %.2f dBTP", *a.Loudness.TruePeak)
		}
		fmt.Fprintf(&b, "\n")
	}
	fmt.Fprintf(&b, "Stereo: Mid RMS %.2f dB | Side RMS %.2f dB | Side/Mid %.2f dB",
		a.Stereo.MidRMS, a.Stereo.SideRMS, a.Stereo.SideMidRatioDB)
	if a.Stereo.Correlation != nil {
		fmt.Fprintf(&b, " | Corr %.2f", *a.Stereo.Correlation)
	}
	fmt.Fprintf(&b, "\n")
	if a.Spectral.Centroid != nil || a.Spectral.Flatness != nil || a.Spectral.Rolloff95 != nil {
		fmt.Fprintf(&b, "Spectral:")
		if a.Spectral.Centroid != nil {
			fmt.Fprintf(&b, " Centroid %.0f Hz", *a.Spectral.Centroid)
		}
		if a.Spectral.Rolloff95 != nil {
			fmt.Fprintf(&b, " | Rolloff95 %.0f Hz", *a.Spectral.Rolloff95)
		}
		if a.Spectral.Flatness != nil {
			fmt.Fprintf(&b, " | Flatness %.3f", *a.Spectral.Flatness)
		}
		if a.Spectral.Spread != nil {
			fmt.Fprintf(&b, " | Spread %.3f", *a.Spectral.Spread)
		}
		if a.Spectral.Skewness != nil {
			fmt.Fprintf(&b, " | Skew %.3f", *a.Spectral.Skewness)
		}
		if a.Spectral.Kurtosis != nil {
			fmt.Fprintf(&b, " | Kurt %.3f", *a.Spectral.Kurtosis)
		}
		fmt.Fprintf(&b, "\n")
	}
	if a.Tempo != nil {
		fmt.Fprintf(&b, "Tempo: ")
		if a.Tempo.BPMMedian != nil {
			fmt.Fprintf(&b, "BPM med %.2f", *a.Tempo.BPMMedian)
		}
		if a.Tempo.BPMMean != nil {
			fmt.Fprintf(&b, " | mean %.2f", *a.Tempo.BPMMean)
		}
		if a.Tempo.BPMStd != nil {
			fmt.Fprintf(&b, " | std %.2f", *a.Tempo.BPMStd)
		}
		fmt.Fprintf(&b, " | events %d", a.Tempo.Events)
		if a.Tempo.OnsetPerMin != nil {
			fmt.Fprintf(&b, " | onsets/min %.2f", *a.Tempo.OnsetPerMin)
		}
		fmt.Fprintf(&b, "\n")
	}
	if a.Pitch != nil && (a.Pitch.HzMedian != nil || a.Pitch.Note != nil) {
		fmt.Fprintf(&b, "Pitch: ")
		if a.Pitch.HzMedian != nil {
			fmt.Fprintf(&b, "median %.2f Hz", *a.Pitch.HzMedian)
		}
		if a.Pitch.HzMean != nil {
			fmt.Fprintf(&b, " | mean %.2f Hz", *a.Pitch.HzMean)
		}
		if a.Pitch.HzMin != nil && a.Pitch.HzMax != nil {
			fmt.Fprintf(&b, " | min/max %.2f/%.2f Hz", *a.Pitch.HzMin, *a.Pitch.HzMax)
		}
		if a.Pitch.MIDIMedian != nil {
			fmt.Fprintf(&b, " | MIDI %.1f", *a.Pitch.MIDIMedian)
		}
		if a.Pitch.Note != nil {
			fmt.Fprintf(&b, " | note %s", *a.Pitch.Note)
		}
		fmt.Fprintf(&b, "\n")
	}
	if a.Key != nil && (a.Key.Key != nil || a.Key.Scale != nil) {
		fmt.Fprintf(&b, "Key: ")
		if a.Key.Key != nil {
			fmt.Fprintf(&b, "%s", *a.Key.Key)
		}
		if a.Key.Scale != nil {
			fmt.Fprintf(&b, " %s", *a.Key.Scale)
		}
		if a.Key.Conf != nil {
			fmt.Fprintf(&b, " (conf %.2f)", *a.Key.Conf)
		}
		fmt.Fprintf(&b, "\n")
	}
	if len(a.Bands) > 0 {
		fmt.Fprintf(&b, "\nBand Loudness (dBFS):\n")
		for _, bs := range a.Bands {
			fmt.Fprintf(&b, "  %6.0f-%-6.0f Hz : peak %7.2f | rms %7.2f\n", bs.Band.Lo, bs.Band.Hi, bs.PeakDB, bs.RMSDB)
		}
	}
	if len(a.Silence) > 0 {
		fmt.Fprintf(&b, "\nSilence spans (threshold ~%.1f dBFS):\n", a.Level.NoiseFloor)
		for _, s := range a.Silence {
			fmt.Fprintf(&b, "  %.3f → %.3f (%.3fs)\n", s.Start, s.End, s.End-s.Start)
		}
		if a.SilenceTotal != nil {
			fmt.Fprintf(&b, "Total silence: %.3fs\n", *a.SilenceTotal)
		}
		if a.SilenceRatio != nil {
			fmt.Fprintf(&b, "Silence ratio: %.2f%% of duration\n", *a.SilenceRatio*100)
		}
	}
	if len(a.Notes) > 0 {
		fmt.Fprintf(&b, "\nNotes:\n")
		for _, n := range a.Notes {
			fmt.Fprintf(&b, "  - %s\n", n)
		}
	}
	return b.String()
}

func renderMD(a *Analysis) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Analysis: %s\n\n", filepath.Base(a.File))
	fmt.Fprintf(&b, "- When: `%s`\n- Format: `%s`\n- Duration: `%.3fs`\n- Sample Rate: `%d Hz`\n- Channels: `%d`\n- Bit Depth: `%d`\n\n",
		a.When, a.Probe.FormatName, a.Probe.Duration, a.Probe.SampleRate, a.Probe.Channels, a.Probe.BitDepth)

	fmt.Fprintf(&b, "## Levels\n")
	fmt.Fprintf(&b, "- Peak: `%.2f dBFS`\n- RMS: `%.2f dBFS`\n- Crest: `%.2f dB`\n- Headroom: `%.2f dB`\n",
		a.Level.PeakDB, a.Level.RMSDB, a.Level.CrestDB, a.Level.HeadroomDB)
	if a.Level.TruePeakDBTP != nil {
		fmt.Fprintf(&b, "- True Peak: `%.2f dBTP`\n", *a.Level.TruePeakDBTP)
	}
	if a.Level.ClipSamples != nil && a.Level.ClipPercent != nil {
		fmt.Fprintf(&b, "- Clipped samples: `%d (%.3f%%)`\n", *a.Level.ClipSamples, *a.Level.ClipPercent)
	}
	fmt.Fprintf(&b, "- DC Offset: `%.4f`\n- Zero-Crossing Rate: `%.2f`\n- Noise Floor: `%.2f dBFS`\n\n",
		a.Level.DCOffset, a.Level.ZeroXRate, a.Level.NoiseFloor)

	if a.Loudness != nil {
		fmt.Fprintf(&b, "## Loudness (EBU R128)\n- Integrated: `%.2f LUFS`\n- Range: `%.2f LU`\n", a.Loudness.Integrated, a.Loudness.Range)
		if a.Loudness.TruePeak != nil {
			fmt.Fprintf(&b, "- True Peak: `%.2f dBTP`\n", *a.Loudness.TruePeak)
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "## Stereo\n- Mid RMS: `%.2f dB`\n- Side RMS: `%.2f dB`\n- Side/Mid: `%.2f dB`\n",
		a.Stereo.MidRMS, a.Stereo.SideRMS, a.Stereo.SideMidRatioDB)
	if a.Stereo.Correlation != nil {
		fmt.Fprintf(&b, "- Correlation: `%.2f`\n", *a.Stereo.Correlation)
	}
	fmt.Fprintf(&b, "\n")

	if a.Spectral.Centroid != nil || a.Spectral.Rolloff95 != nil || a.Spectral.Flatness != nil {
		fmt.Fprintf(&b, "## Spectral\n")
		if a.Spectral.Centroid != nil {
			fmt.Fprintf(&b, "- Centroid: `%.0f Hz`\n", *a.Spectral.Centroid)
		}
		if a.Spectral.Rolloff95 != nil {
			fmt.Fprintf(&b, "- Rolloff (95%%): `%.0f Hz`\n", *a.Spectral.Rolloff95)
		}
		if a.Spectral.Flatness != nil {
			fmt.Fprintf(&b, "- Flatness: `%.3f`\n", *a.Spectral.Flatness)
		}
		if a.Spectral.Spread != nil {
			fmt.Fprintf(&b, "- Spread: `%.3f`\n", *a.Spectral.Spread)
		}
		if a.Spectral.Skewness != nil {
			fmt.Fprintf(&b, "- Skewness: `%.3f`\n", *a.Spectral.Skewness)
		}
		if a.Spectral.Kurtosis != nil {
			fmt.Fprintf(&b, "- Kurtosis: `%.3f`\n", *a.Spectral.Kurtosis)
		}
		fmt.Fprintf(&b, "\n")
	}

	if a.Tempo != nil {
		fmt.Fprintf(&b, "## Tempo\n")
		if a.Tempo.BPMMedian != nil {
			fmt.Fprintf(&b, "- BPM (median): `%.2f`\n", *a.Tempo.BPMMedian)
		}
		if a.Tempo.BPMMean != nil {
			fmt.Fprintf(&b, "- BPM (mean): `%.2f`\n", *a.Tempo.BPMMean)
		}
		if a.Tempo.BPMStd != nil {
			fmt.Fprintf(&b, "- BPM (stddev): `%.2f`\n", *a.Tempo.BPMStd)
		}
		fmt.Fprintf(&b, "- Tempo events: `%d`\n", a.Tempo.Events)
		if a.Tempo.OnsetPerMin != nil {
			fmt.Fprintf(&b, "- Onsets/min: `%.2f`\n", *a.Tempo.OnsetPerMin)
		}
		fmt.Fprintf(&b, "\n")
	}

	if a.Pitch != nil && (a.Pitch.HzMedian != nil || a.Pitch.Note != nil) {
		fmt.Fprintf(&b, "## Pitch\n")
		if a.Pitch.HzMedian != nil {
			fmt.Fprintf(&b, "- Median: `%.2f Hz`\n", *a.Pitch.HzMedian)
		}
		if a.Pitch.HzMean != nil {
			fmt.Fprintf(&b, "- Mean: `%.2f Hz`\n", *a.Pitch.HzMean)
		}
		if a.Pitch.HzMin != nil && a.Pitch.HzMax != nil {
			fmt.Fprintf(&b, "- Min/Max: `%.2f / %.2f Hz`\n", *a.Pitch.HzMin, *a.Pitch.HzMax)
		}
		if a.Pitch.MIDIMedian != nil {
			fmt.Fprintf(&b, "- MIDI: `%.1f`\n", *a.Pitch.MIDIMedian)
		}
		if a.Pitch.Note != nil {
			fmt.Fprintf(&b, "- Note: `%s`\n", *a.Pitch.Note)
		}
		fmt.Fprintf(&b, "\n")
	}

	if a.Key != nil && (a.Key.Key != nil || a.Key.Scale != nil) {
		fmt.Fprintf(&b, "## Key\n")
		if a.Key.Key != nil {
			fmt.Fprintf(&b, "- Key: `%s`\n", *a.Key.Key)
		}
		if a.Key.Scale != nil {
			fmt.Fprintf(&b, "- Scale: `%s`\n", *a.Key.Scale)
		}
		if a.Key.Conf != nil {
			fmt.Fprintf(&b, "- Confidence: `%.2f`\n", *a.Key.Conf)
		}
		fmt.Fprintf(&b, "\n")
	}

	if len(a.Bands) > 0 {
		fmt.Fprintf(&b, "## Band Loudness\n\n| Band (Hz) | Peak (dBFS) | RMS (dBFS) |\n|---:|---:|---:|\n")
		for _, bs := range a.Bands {
			fmt.Fprintf(&b, "| %.0f–%.0f | %.2f | %.2f |\n", bs.Band.Lo, bs.Band.Hi, bs.PeakDB, bs.RMSDB)
		}
		fmt.Fprintf(&b, "\n")
	}

	if len(a.Silence) > 0 {
		fmt.Fprintf(&b, "## Silence\n")
		for _, s := range a.Silence {
			fmt.Fprintf(&b, "- `%.3f → %.3f` (%.3fs)\n", s.Start, s.End, s.End-s.Start)
		}
		if a.SilenceTotal != nil {
			fmt.Fprintf(&b, "- Total silence: `%.3fs`\n", *a.SilenceTotal)
		}
		if a.SilenceRatio != nil {
			fmt.Fprintf(&b, "- Silence ratio: `%.2f%%`\n", *a.SilenceRatio*100)
		}
		fmt.Fprintf(&b, "\n")
	}

	if len(a.Notes) > 0 {
		fmt.Fprintf(&b, "## Notes\n")
		for _, n := range a.Notes {
			fmt.Fprintf(&b, "- %s\n", n)
		}
		fmt.Fprintf(&b, "\n")
	}
	return b.String()
}

func renderDiff(cfg *Config, d *Diff) string {
	switch strings.ToLower(cfg.Report) {
	case "json":
		buf, _ := json.MarshalIndent(d, "", "  ")
		return string(buf) + "\n"
	case "md":
		var b strings.Builder
		fmt.Fprintf(&b, "# Compare: %s ↔ %s\n\n", filepath.Base(d.A.File), filepath.Base(d.B.File))
		fmt.Fprintf(&b, "| Metric | %s | %s | Δ (B-A) |\n|---|---:|---:|---:|\n", filepath.Base(d.A.File), filepath.Base(d.B.File))
		row := func(name string, av, bv, dv float64, fmtStr string) {
			fmt.Fprintf(&b, "| %s | "+fmtStr+" | "+fmtStr+" | "+fmtStr+" |\n", name, av, bv, dv)
		}
		row("Peak dBFS", d.A.Level.PeakDB, d.B.Level.PeakDB, d.Delta["peak_db"], "%.2f")
		row("RMS dBFS", d.A.Level.RMSDB, d.B.Level.RMSDB, d.Delta["rms_db"], "%.2f")
		row("Crest dB", d.A.Level.CrestDB, d.B.Level.CrestDB, d.Delta["crest_db"], "%.2f")
		if d.A.Loudness != nil && d.B.Loudness != nil {
			row("LUFS (integr.)", d.A.Loudness.Integrated, d.B.Loudness.Integrated, d.Delta["lufs_integrated"], "%.2f")
			row("LUFS Range", d.A.Loudness.Range, d.B.Loudness.Range, d.Delta["lufs_range"], "%.2f")
		}
		row("Side/Mid dB", d.A.Stereo.SideMidRatioDB, d.B.Stereo.SideMidRatioDB, d.Delta["stereo_side_mid_db"], "%.2f")
		if d.A.Tempo != nil && d.B.Tempo != nil && d.A.Tempo.BPMMedian != nil && d.B.Tempo.BPMMedian != nil {
			row("BPM (median)", *d.A.Tempo.BPMMedian, *d.B.Tempo.BPMMedian, d.Delta["bpm_median"], "%.2f")
		}
		row("Duration (s)", d.A.Probe.Duration, d.B.Probe.Duration, d.Delta["duration_s"], "%.3f")
		return b.String()
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "COMPARE: %s vs %s\n\n", d.A.File, d.B.File)
		for _, k := range []string{"peak_db", "rms_db", "crest_db", "lufs_integrated", "lufs_range", "stereo_side_mid_db", "bpm_median", "duration_s"} {
			if v, ok := d.Delta[k]; ok && !math.IsNaN(v) && !math.IsInf(v, 0) {
				fmt.Fprintf(&b, "%-20s : %+8.3f\n", k, v)
			}
		}
		return b.String()
	}
}
