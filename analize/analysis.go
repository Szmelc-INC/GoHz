package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func analyzeFile(cfg *Config, in string) (*Analysis, error) {
	if _, err := os.Stat(in); err != nil {
		return nil, err
	}
	probe, err := ffprobeInfo(cfg, in)
	if err != nil {
		return nil, err
	}

	peak, rms, _ := ffmpegVolumedetect(cfg, in)
	astatsMap, _ := ffmpegAstatsOverall(cfg, in, cfg.AstatsWin)
	lv := LevelStats{
		PeakDB: peak, RMSDB: rms, CrestDB: peak - rms,
		DCOffset: astatsMap["dc_offset"], ZeroXRate: astatsMap["zero_crossings_rate"],
		NoiseFloor: astatsMap["noise_floor"],
	}
	if v, ok := astatsMap["number_of_clipped_samples"]; ok {
		c := int64(v)
		lv.ClipSamples = &c
		if probe.Duration > 0 && probe.SampleRate > 0 && probe.Channels > 0 {
			total := probe.Duration * float64(probe.SampleRate*probe.Channels)
			p := 100.0 * float64(c) / total
			lv.ClipPercent = &p
		}
	}
	lv.HeadroomDB = 0 - lv.PeakDB

	var lufs *LUFS
	if cfg.UseEBUR128 {
		if v, err := ffmpegEBUR128(cfg, in); err == nil {
			lufs = &v
			if v.TruePeak != nil {
				lv.TruePeakDBTP = v.TruePeak
			}
		}
	}

	spec, _ := ffmpegSpectral(cfg, in)
	st, _ := ffmpegStereoStuff(cfg, in)

	var bands []BandStat
	if cfg.UseBands {
		for _, b := range cfg.Bands {
			if p, r, err := ffmpegBandLoudness(cfg, in, b); err == nil {
				bands = append(bands, BandStat{Band: b, PeakDB: p, RMSDB: r})
			}
		}
	}

	sil, _ := detectSilences(cfg, in)
	var silRatio *float64
	var silTotal *float64
	if len(sil) > 0 {
		var dur float64
		for _, sp := range sil {
			dur += sp.End - sp.Start
		}
		silTotal = &dur
		if probe.Duration > 0 {
			v := dur / probe.Duration
			silRatio = &v
		}
	}

	var tempo *TempoStats
	if strings.ToLower(cfg.BPMEngine) == "aubio" {
		if series, err := aubioBPMSeries(cfg, in); err == nil {
			med := series[len(series)/2]
			mu := mean(series)
			sd := stddev(series, mu)
			onr, events, _ := aubioOnsetRate(cfg, in, probe.Duration)
			tempo = &TempoStats{
				BPMMedian: &med, BPMMean: &mu, BPMStd: &sd, Events: events, OnsetPerMin: onr,
			}
		}
	}

	ps, _ := aubioPitchStats(cfg, in)
	var key *KeyInfo
	if k, err := aubioKey(cfg, in); err == nil {
		key = k
	}

	var notes []string
	if lv.ClipSamples != nil && *lv.ClipSamples > 0 {
		notes = append(notes, fmt.Sprintf("Clipping detected: %d samples (%.3f%%)", *lv.ClipSamples, derefFloat(lv.ClipPercent)))
	}
	if lv.TruePeakDBTP != nil && *lv.TruePeakDBTP > -1.0 {
		notes = append(notes, fmt.Sprintf("True peak dangerously high (%.2f dBTP). Consider -1.5 dBTP ceiling.", *lv.TruePeakDBTP))
	}
	if spec.Flatness != nil && *spec.Flatness > 0.5 {
		notes = append(notes, "High spectral flatness → noise-like content.")
	}
	if st.Correlation != nil && *st.Correlation < 0.2 {
		notes = append(notes, "Low L/R correlation → wide or phasey stereo.")
	}

	return &Analysis{
		File: in, When: time.Now().Format(time.RFC3339),
		Probe: probe, Level: lv, Loudness: lufs, Stereo: st, Spectral: spec,
		Bands: bands, Tempo: tempo, Pitch: ps, Key: key,
		Silence: sil, SilenceRatio: silRatio, SilenceTotal: silTotal, Notes: notes,
	}, nil
}
