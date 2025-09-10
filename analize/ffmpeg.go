package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

func ffprobeInfo(cfg *Config, in string) (ProbeInfo, error) {
	args := []string{"-v", "error", "-show_format", "-show_streams", "-of", "json", in}
	out, err := runCmd(cfg.FFprobeBin, args...)
	if err != nil {
		return ProbeInfo{}, fmt.Errorf("ffprobe: %v", err)
	}
	type ffFmt struct {
		Format struct {
			FormatName string `json:"format_name"`
			Duration   string `json:"duration"`
			BitRate    string `json:"bit_rate"`
		} `json:"format"`
		Streams []struct {
			CodecType        string `json:"codec_type"`
			SampleRate       string `json:"sample_rate"`
			Channels         int    `json:"channels"`
			BitsPerRawSample string `json:"bits_per_raw_sample"`
			BitsPerSample    int    `json:"bits_per_sample"`
		} `json:"streams"`
	}
	var ff ffFmt
	if err := json.Unmarshal([]byte(out), &ff); err != nil {
		return ProbeInfo{}, err
	}
	p := ProbeInfo{
		FormatName: ff.Format.FormatName,
		Duration:   parseFloat(ff.Format.Duration),
		BitRate:    int64(parseInt(ff.Format.BitRate)),
	}
	for _, s := range ff.Streams {
		if s.CodecType == "audio" {
			p.SampleRate = parseInt(s.SampleRate)
			p.Channels = s.Channels
			if s.BitsPerSample > 0 {
				p.BitDepth = s.BitsPerSample
			} else if s.BitsPerRawSample != "" {
				p.BitDepth = parseInt(s.BitsPerRawSample)
			}
			break
		}
	}
	return p, nil
}

func ffmpegVolumedetect(cfg *Config, in string) (peakDB, rmsDB float64, err error) {
	args := []string{"-hide_banner", "-nostats", "-vn", "-i", in, "-af", "volumedetect", "-f", "null", "-"}
	out, _ := runCmd(cfg.FFmpegBin, args...)
	reMax := regexp.MustCompile(`max_volume:\s*([-\d\.]+)\s*dB`)
	reMean := regexp.MustCompile(`mean_volume:\s*([-\d\.]+)\s*dB`)
	m1 := reMax.FindStringSubmatch(out)
	m2 := reMean.FindStringSubmatch(out)
	if len(m1) < 2 || len(m2) < 2 {
		return 0, 0, fmt.Errorf("volumedetect parse failed")
	}
	return parseFloat(m1[1]), parseFloat(m2[1]), nil
}

// generic astats parser (overall)
func ffmpegAstatsOverall(cfg *Config, in string, windowSec float64) (map[string]float64, error) {
	filter := "astats=measure_overall=1:reset=0"
	if windowSec > 0 {
		filter = fmt.Sprintf("astats=measure_overall=1:metadata=1:reset=1:window=%0.2f", windowSec)
	}
	args := []string{"-hide_banner", "-nostats", "-vn", "-i", in, "-af", filter, "-f", "null", "-"}
	out, _ := runCmd(cfg.FFmpegBin, args...)
	re := regexp.MustCompile(`Overall ([A-Za-z0-9 /\-]+):\s*([-\d\.]+)`)
	stats := map[string]float64{}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if m := re.FindStringSubmatch(line); len(m) == 3 {
			key := strings.TrimSpace(strings.ToLower(strings.ReplaceAll(m[1], " ", "_")))
			stats[key] = parseFloat(m[2])
		}
	}
	if len(stats) == 0 {
		return stats, fmt.Errorf("no astats parsed")
	}
	return stats, nil
}

func ffmpegEBUR128(cfg *Config, in string) (LUFS, error) {
	args := []string{"-hide_banner", "-nostats", "-vn", "-i", in, "-filter_complex", "ebur128=peak=true", "-f", "null", "-"}
	out, _ := runCmd(cfg.FFmpegBin, args...)
	reI := regexp.MustCompile(`Integrated loudness:\s*([-\d\.]+)\s*LUFS`)
	reR := regexp.MustCompile(`Loudness range:\s*([-\d\.]+)\s*LU`)
	reTP := regexp.MustCompile(`True peak:\s*([-\d\.]+)\s*dBTP`)
	mI := reI.FindStringSubmatch(out)
	mR := reR.FindStringSubmatch(out)
	l := LUFS{}
	if len(mI) >= 2 {
		l.Integrated = parseFloat(mI[1])
	} else {
		return l, fmt.Errorf("no integrated")
	}
	if len(mR) >= 2 {
		l.Range = parseFloat(mR[1])
	}
	if m := reTP.FindStringSubmatch(out); len(m) >= 2 {
		v := parseFloat(m[1])
		l.TruePeak = &v
	}
	return l, nil
}

func ffmpegBandLoudness(cfg *Config, in string, b Bandspec) (peakDB, rmsDB float64, err error) {
	filter := fmt.Sprintf("highpass=f=%g,lowpass=f=%g,volumedetect", b.Lo, b.Hi)
	args := []string{"-hide_banner", "-nostats", "-vn", "-i", in, "-af", filter, "-f", "null", "-"}
	out, _ := runCmd(cfg.FFmpegBin, args...)
	reMax := regexp.MustCompile(`max_volume:\s*([-\d\.]+)\s*dB`)
	reMean := regexp.MustCompile(`mean_volume:\s*([-\d\.]+)\s*dB`)
	m1 := reMax.FindStringSubmatch(out)
	m2 := reMean.FindStringSubmatch(out)
	if len(m1) < 2 || len(m2) < 2 {
		return 0, 0, fmt.Errorf("band parse failed")
	}
	return parseFloat(m1[1]), parseFloat(m2[1]), nil
}

// mid/side + correlation (if available)
func ffmpegStereoStuff(cfg *Config, in string) (StereoStats, error) {
	filter := "asplit=2[a][b];" +
		"[a]channelsplit=channel_layout=stereo:channels=FL|FR[aL][aR];" +
		"[aL][aR]join=inputs=2:channel_layout=stereo,pan=stereo|c0=0.5*FL+0.5*FR|c1=0.5*FL+0.5*FR[mid2];" +
		"[b]channelsplit=channel_layout=stereo:channels=FL|FR[bL][bR];" +
		"[bL][bR]join=inputs=2:channel_layout=stereo,pan=stereo|c0=0.5*FL-0.5*FR|c1=0.5*FL-0.5*FR[side2];" +
		"[0:a]astats=measure_overall=1:reset=0[origstats];" +
		"[mid2]astats=measure_overall=1:reset=0[midstats];" +
		"[side2]astats=measure_overall=1:reset=0[sidestats]"
	args := []string{"-hide_banner", "-nostats", "-vn", "-i", in, "-filter_complex", filter, "-f", "null", "-"}
	out, _ := runCmd(cfg.FFmpegBin, args...)
	reRMS := regexp.MustCompile(`\[Parsed_astats.*\] Overall RMS level:\s*([-\d\.]+)`)
	var vals []float64
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if m := reRMS.FindStringSubmatch(line); len(m) == 2 {
			vals = append(vals, parseFloat(m[1]))
		}
	}
	var mid, side float64
	if len(vals) >= 4 {
		mid = (vals[2] + vals[3]) / 2
		side = (vals[0] + vals[1]) / 2
	} else if len(vals) >= 2 {
		mid = vals[0]
		side = vals[1]
	}
	var corr *float64
	reCorr := regexp.MustCompile(`Overall channel correlation:\s*([-\d\.]+)`)
	if m := reCorr.FindStringSubmatch(out); len(m) == 2 {
		v := parseFloat(m[1])
		corr = &v
	}
	return StereoStats{
		MidRMS:         mid,
		SideRMS:        side,
		SideMidRatioDB: side - mid,
		Correlation:    corr,
	}, nil
}

// spectral goodies from astats overall
func ffmpegSpectral(cfg *Config, in string) (SpectralStats, error) {
	args := []string{"-hide_banner", "-nostats", "-vn", "-i", in, "-af", "astats=measure_overall=1:reset=0", "-f", "null", "-"}
	out, _ := runCmd(cfg.FFmpegBin, args...)
	get := func(name string) *float64 {
		re := regexp.MustCompile(fmt.Sprintf(`Overall %s:\s*([-\d\.]+)`, regexp.QuoteMeta(name)))
		if m := re.FindStringSubmatch(out); len(m) == 2 {
			v := parseFloat(m[1])
			return &v
		}
		return nil
	}
	return SpectralStats{
		Centroid:  get("spectral centroid"),
		Rolloff95: get("spectral rolloff"),
		Flatness:  get("flat factor"),
		Spread:    get("spectral spread"),
		Skewness:  get("spectral skewness"),
		Kurtosis:  get("spectral kurtosis"),
	}, nil
}

// silence spans
func detectSilences(cfg *Config, in string) ([]SilenceSpan, error) {
	filter := fmt.Sprintf("silencedetect=noise=%0.1fdB:d=0.3", cfg.SilThresDB)
	args := []string{"-hide_banner", "-vn", "-i", in, "-af", filter, "-f", "null", "-"}
	out, _ := runCmd(cfg.FFmpegBin, args...)
	var spans []SilenceSpan
	reS := regexp.MustCompile(`silence_start:\s*([-\d\.]+)`)
	reE := regexp.MustCompile(`silence_end:\s*([-\d\.]+)`)
	var start *float64
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if m := reS.FindStringSubmatch(line); len(m) == 2 {
			v := parseFloat(m[1])
			start = &v
		}
		if m := reE.FindStringSubmatch(line); len(m) == 2 && start != nil {
			end := parseFloat(m[1])
			spans = append(spans, SilenceSpan{*start, end})
			start = nil
		}
	}
	return spans, nil
}
