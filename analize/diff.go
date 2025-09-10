package main

func compare(a, b *Analysis) *Diff {
	d := &Diff{A: a, B: b, Delta: map[string]float64{}}
	d.Delta["peak_db"] = b.Level.PeakDB - a.Level.PeakDB
	d.Delta["rms_db"] = b.Level.RMSDB - a.Level.RMSDB
	d.Delta["crest_db"] = b.Level.CrestDB - a.Level.CrestDB
	if a.Loudness != nil && b.Loudness != nil {
		d.Delta["lufs_integrated"] = b.Loudness.Integrated - a.Loudness.Integrated
		d.Delta["lufs_range"] = b.Loudness.Range - a.Loudness.Range
	}
	d.Delta["stereo_side_mid_db"] = b.Stereo.SideMidRatioDB - a.Stereo.SideMidRatioDB
	if a.Tempo != nil && b.Tempo != nil && a.Tempo.BPMMedian != nil && b.Tempo.BPMMedian != nil {
		d.Delta["bpm_median"] = *b.Tempo.BPMMedian - *a.Tempo.BPMMedian
	}
	d.Delta["duration_s"] = b.Probe.Duration - a.Probe.Duration
	return d
}
