// analit — ridiculous audio analyzer
// Requires: ffmpeg, ffprobe. Optional: aubio (tempo|pitch|key|onset).
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Bandspec struct{ Lo, Hi float64 }

type Config struct {
	// IO / tools
	OutPath     string
	Report      string // txt|json|md
	FFmpegBin   string
	FFprobeBin  string
	AubioBin    string

	// engines
	BPMEngine   string // aubio|none
	UseBands    bool
	Bands       []Bandspec
	UseEBUR128  bool

	// tuning
	AstatsWin   float64
	SilThresDB  float64
}

func defaultConfig() *Config {
	return &Config{
		OutPath:    "out.log",
		Report:     "txt",
		FFmpegBin:  "ffmpeg",
		FFprobeBin: "ffprobe",
		AubioBin:   "aubio",
		BPMEngine:  "none",
		UseBands:   true,
		Bands:      parseBands("20-60,60-120,120-250,250-500,500-2000,2000-5000,5000-10000,10000-20000"),
		UseEBUR128: true,
		AstatsWin:  0,
		SilThresDB: -45,
	}
}

func parseBands(s string) []Bandspec {
	var out []Bandspec
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" { continue }
		chunks := strings.Split(part, "-")
		if len(chunks) != 2 { continue }
		lo, _ := strconv.ParseFloat(strings.TrimSpace(chunks[0]), 64)
		hi, _ := strconv.ParseFloat(strings.TrimSpace(chunks[1]), 64)
		if lo > 0 && hi > lo {
			out = append(out, Bandspec{lo, hi})
		}
	}
	return out
}

// ---------- Models ----------

type ProbeInfo struct {
	FormatName string
	Duration   float64
	SampleRate int
	Channels   int
	BitRate    int64
	BitDepth   int
}

type LevelStats struct {
	PeakDB       float64
	RMSDB        float64
	CrestDB      float64
	TruePeakDBTP *float64
	HeadroomDB   float64
	DCOffset     float64
	ZeroXRate    float64
	NoiseFloor   float64
	ClipSamples  *int64
	ClipPercent  *float64
}

type LUFS struct {
	Integrated float64
	Range      float64
	TruePeak   *float64
}

type BandStat struct {
	Band   Bandspec
	PeakDB float64
	RMSDB  float64
}

type StereoStats struct {
	MidRMS          float64
	SideRMS         float64
	SideMidRatioDB  float64
	Correlation     *float64
}

type SpectralStats struct {
	Centroid   *float64 // Hz (proxy)
	Rolloff95  *float64 // Hz
	Flatness   *float64 // 0..1
	Spread     *float64
	Skewness   *float64
	Kurtosis   *float64
}

type TempoStats struct {
	BPMMedian *float64
	BPMMean   *float64
	BPMStd    *float64
	Events    int
	OnsetPerMin *float64
}

type PitchStats struct {
	HzMedian *float64
	HzMean   *float64
	HzMin    *float64
	HzMax    *float64
	MIDIMedian *float64
	Note     *string // e.g. "A#3"
}

type KeyInfo struct {
	Key   *string // e.g., "C"
	Scale *string // "major" / "minor" / etc.
	Conf  *float64
}

type SilenceSpan struct {
	Start float64
	End   float64
}

type Analysis struct {
	File        string
	When        string
	Probe       ProbeInfo
	Level       LevelStats
	Loudness    *LUFS
	Stereo      StereoStats
	Spectral    SpectralStats
	Bands       []BandStat
	Tempo       *TempoStats
	Pitch       *PitchStats
	Key         *KeyInfo
	Silence     []SilenceSpan
	SilenceRatio *float64
	Notes       []string // warnings/suggestions
}

// ---------- Utils ----------

func fail(fmtStr string, a ...any) {
	fmt.Fprintf(os.Stderr, "[-] "+fmtStr+"\n", a...)
	os.Exit(1)
}
func mustHave(bin string) error { _, err := exec.LookPath(bin); return err }
func runCmd(bin string, args ...string) (string, error) {
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	return string(out), err
}
func parseInt(s string) int { i, _ := strconv.Atoi(strings.TrimSpace(s)); return i }
func parseInt64(s string) int64 { v, _ := strconv.ParseInt(strings.TrimSpace(s),10,64); return v }
func parseFloat(s string) float64 { f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64); return f }
func clamp01(x float64) float64 { if x<0 {return 0}; if x>1 {return 1}; return x }

// ---------- Probing ----------

func ffprobeInfo(cfg *Config, in string) (ProbeInfo, error) {
	args := []string{"-v","error","-show_format","-show_streams","-of","json", in}
	out, err := runCmd(cfg.FFprobeBin, args...)
	if err != nil { return ProbeInfo{}, fmt.Errorf("ffprobe: %v", err) }
	type ffFmt struct {
		Format struct {
			FormatName string `json:"format_name"`
			Duration   string `json:"duration"`
			BitRate    string `json:"bit_rate"`
		} `json:"format"`
		Streams []struct {
			CodecType  string `json:"codec_type"`
			SampleRate string `json:"sample_rate"`
			Channels   int    `json:"channels"`
			BitsPerRawSample string `json:"bits_per_raw_sample"`
			BitsPerSample    int    `json:"bits_per_sample"`
		} `json:"streams"`
	}
	var ff ffFmt
	if err := json.Unmarshal([]byte(out), &ff); err != nil { return ProbeInfo{}, err }
	p := ProbeInfo{
		FormatName: ff.Format.FormatName,
		Duration:   parseFloat(ff.Format.Duration),
		BitRate:    int64(parseInt(ff.Format.BitRate)),
	}
	for _, s := range ff.Streams {
		if s.CodecType == "audio" {
			p.SampleRate = parseInt(s.SampleRate)
			p.Channels = s.Channels
			if s.BitsPerSample > 0 { p.BitDepth = s.BitsPerSample
			} else if s.BitsPerRawSample != "" { p.BitDepth = parseInt(s.BitsPerRawSample) }
			break
		}
	}
	return p, nil
}

func ffmpegVolumedetect(cfg *Config, in string) (peakDB, rmsDB float64, err error) {
	args := []string{"-hide_banner","-nostats","-vn","-i", in, "-af", "volumedetect", "-f","null","-"}
	out, _ := runCmd(cfg.FFmpegBin, args...)
	reMax := regexp.MustCompile(`max_volume:\s*([-\d\.]+)\s*dB`)
	reMean:= regexp.MustCompile(`mean_volume:\s*([-\d\.]+)\s*dB`)
	m1 := reMax.FindStringSubmatch(out); m2 := reMean.FindStringSubmatch(out)
	if len(m1)<2 || len(m2)<2 { return 0,0, fmt.Errorf("volumedetect parse failed") }
	return parseFloat(m1[1]), parseFloat(m2[1]), nil
}

// generic astats parser (overall)
func ffmpegAstatsOverall(cfg *Config, in string, windowSec float64) (map[string]float64, error) {
	filter := "astats=measure_overall=1:reset=0"
	if windowSec > 0 {
		filter = fmt.Sprintf("astats=measure_overall=1:metadata=1:reset=1:window=%0.2f", windowSec)
	}
	args := []string{"-hide_banner","-nostats","-vn","-i", in, "-af", filter, "-f","null","-"}
	out, _ := runCmd(cfg.FFmpegBin, args...)
	// capture "Overall XXX: value"
	re := regexp.MustCompile(`Overall ([A-Za-z0-9 /\-]+):\s*([-\d\.]+)`)
	stats := map[string]float64{}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if m := re.FindStringSubmatch(line); len(m)==3 {
			key := strings.TrimSpace(strings.ToLower(strings.ReplaceAll(m[1], " ", "_")))
			stats[key] = parseFloat(m[2])
		}
	}
	if len(stats)==0 { return stats, fmt.Errorf("no astats parsed") }
	return stats, nil
}

func ffmpegEBUR128(cfg *Config, in string) (LUFS, error) {
	args := []string{"-hide_banner","-nostats","-vn","-i", in, "-filter_complex","ebur128=peak=true","-f","null","-"}
	out, _ := runCmd(cfg.FFmpegBin, args...)
	reI := regexp.MustCompile(`Integrated loudness:\s*([-\d\.]+)\s*LUFS`)
	reR := regexp.MustCompile(`Loudness range:\s*([-\d\.]+)\s*LU`)
	reTP:= regexp.MustCompile(`True peak:\s*([-\d\.]+)\s*dBTP`)
	mI := reI.FindStringSubmatch(out)
	mR := reR.FindStringSubmatch(out)
	l := LUFS{}
	if len(mI)>=2 { l.Integrated = parseFloat(mI[1]) } else { return l, fmt.Errorf("no integrated") }
	if len(mR)>=2 { l.Range = parseFloat(mR[1]) }
	if m := reTP.FindStringSubmatch(out); len(m)>=2 {
		v := parseFloat(m[1]); l.TruePeak = &v
	}
	return l, nil
}

func ffmpegBandLoudness(cfg *Config, in string, b Bandspec) (peakDB, rmsDB float64, err error) {
	filter := fmt.Sprintf("highpass=f=%g,lowpass=f=%g,volumedetect", b.Lo, b.Hi)
	args := []string{"-hide_banner","-nostats","-vn","-i", in, "-af", filter, "-f","null","-"}
	out, _ := runCmd(cfg.FFmpegBin, args...)
	reMax := regexp.MustCompile(`max_volume:\s*([-\d\.]+)\s*dB`)
	reMean:= regexp.MustCompile(`mean_volume:\s*([-\d\.]+)\s*dB`)
	m1 := reMax.FindStringSubmatch(out); m2 := reMean.FindStringSubmatch(out)
	if len(m1)<2 || len(m2)<2 { return 0,0, fmt.Errorf("band parse failed") }
	return parseFloat(m1[1]), parseFloat(m2[1]), nil
}

// mid/side + correlation (if available)
func ffmpegStereoStuff(cfg *Config, in string) (StereoStats, error) {
	// mid/side RMS + correlation via astats on original stereo
	// mid/side as before, correlation from astats "Overall channel correlation"
	filter := "asplit=2[a][b];" +
		"[a]channelsplit=channel_layout=stereo:channels=FL|FR[aL][aR];" +
		"[aL][aR]join=inputs=2:channel_layout=stereo,pan=stereo|c0=0.5*FL+0.5*FR|c1=0.5*FL+0.5*FR[mid2];" +
		"[b]channelsplit=channel_layout=stereo:channels=FL|FR[bL][bR];" +
		"[bL][bR]join=inputs=2:channel_layout=stereo,pan=stereo|c0=0.5*FL-0.5*FR|c1=0.5*FL-0.5*FR[side2];" +
		"[0:a]astats=measure_overall=1:reset=0[origstats];" +
		"[mid2]astats=measure_overall=1:reset=0[midstats];" +
		"[side2]astats=measure_overall=1:reset=0[sidestats]"
	args := []string{"-hide_banner","-nostats","-vn","-i", in, "-filter_complex", filter, "-f","null","-"}
	out, _ := runCmd(cfg.FFmpegBin, args...)
	reRMS := regexp.MustCompile(`\[Parsed_astats.*\] Overall RMS level:\s*([-\d\.]+)`)
	var vals []float64
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if m := reRMS.FindStringSubmatch(line); len(m)==2 {
			vals = append(vals, parseFloat(m[1]))
		}
	}
	var mid, side float64
	if len(vals) >= 4 {
		mid = (vals[2] + vals[3]) / 2 // careful: order depends on parse; last two correspond to sidestats often
		side = (vals[0] + vals[1]) / 2
	} else if len(vals) >= 2 {
		mid = vals[0]; side = vals[1]
	}
	// correlation (orig)
	var corr *float64
	reCorr := regexp.MustCompile(`Overall channel correlation:\s*([-\d\.]+)`)
	if m := reCorr.FindStringSubmatch(out); len(m)==2 {
		v := parseFloat(m[1]); corr = &v
	}
	return StereoStats{
		MidRMS: mid,
		SideRMS: side,
		SideMidRatioDB: side - mid,
		Correlation: corr,
	}, nil
}

// spectral goodies from astats overall
func ffmpegSpectral(cfg *Config, in string) (SpectralStats, error) {
	args := []string{"-hide_banner","-nostats","-vn","-i", in, "-af","astats=measure_overall=1:reset=0","-f","null","-"}
	out, _ := runCmd(cfg.FFmpegBin, args...)
	get := func(name string) *float64 {
		re := regexp.MustCompile(fmt.Sprintf(`Overall %s:\s*([-\d\.]+)`, regexp.QuoteMeta(name)))
		if m := re.FindStringSubmatch(out); len(m)==2 {
			v := parseFloat(m[1]); return &v
		}
		return nil
	}
	return SpectralStats{
		Centroid:  get("spectral centroid"),
		Rolloff95: get("spectral rolloff"),
		Flatness:  get("flat factor"), // sometimes "flat factor" or "spectral flatness"
		Spread:    get("spectral spread"),
		Skewness:  get("spectral skewness"),
		Kurtosis:  get("spectral kurtosis"),
	}, nil
}

// silence spans
func detectSilences(cfg *Config, in string) ([]SilenceSpan, error) {
	filter := fmt.Sprintf("silencedetect=noise=%0.1fdB:d=0.3", cfg.SilThresDB)
	args := []string{"-hide_banner","-vn","-i", in, "-af", filter, "-f","null","-"}
	out, _ := runCmd(cfg.FFmpegBin, args...)
	var spans []SilenceSpan
	reS := regexp.MustCompile(`silence_start:\s*([-\d\.]+)`)
	reE := regexp.MustCompile(`silence_end:\s*([-\d\.]+)`)
	var start *float64
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if m := reS.FindStringSubmatch(line); len(m)==2 { v:=parseFloat(m[1]); start=&v }
		if m := reE.FindStringSubmatch(line); len(m)==2 && start!=nil {
			end := parseFloat(m[1]); spans = append(spans, SilenceSpan{*start, end}); start=nil
		}
	}
	return spans, nil
}

// ---------- AUBIO helpers ----------

func aubioBPMSeries(cfg *Config, in string) ([]float64, error) {
	if err := mustHave(cfg.AubioBin); err != nil { return nil, errors.New("aubio not found") }
	out, err := runCmd(cfg.AubioBin, "tempo", "-i", in)
	if err != nil && out=="" { return nil, fmt.Errorf("aubio tempo failed: %v", err) }
	re := regexp.MustCompile(`([0-9]+(\.[0-9]+)?)\s*bpm`)
	var vals []float64
	sc := bufio.NewScanner(strings.NewReader(strings.ToLower(out)))
	for sc.Scan() {
		if m := re.FindStringSubmatch(sc.Text()); len(m)>=2 {
			vals = append(vals, parseFloat(m[1]))
		}
	}
	if len(vals)==0 { return nil, fmt.Errorf("no bpm series") }
	return vals, nil
}

func aubioOnsetRate(cfg *Config, in string, durSec float64) (*float64, int, error) {
	if err := mustHave(cfg.AubioBin); err != nil { return nil, 0, errors.New("aubio not found") }
	out, _ := runCmd(cfg.AubioBin, "onset", "-i", in)
	// each line usually an onset timestamp
	var count int
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line=="" { continue }
		// usually "t s" numbers; be generous
		if _, err := strconv.ParseFloat(strings.Fields(line)[0], 64); err==nil {
			count++
		}
	}
	if durSec <= 0 { return nil, count, nil }
	rate := float64(count) / (durSec/60.0)
	return &rate, count, nil
}

func aubioPitchStats(cfg *Config, in string) (*PitchStats, error) {
	if err := mustHave(cfg.AubioBin); err != nil { return nil, errors.New("aubio not found") }
	out, _ := runCmd(cfg.AubioBin, "pitch", "-i", in)
	re := regexp.MustCompile(`([0-9]+(\.[0-9]+)?)`)
	var Hz []float64
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		// aubio pitch prints: "<time> <freqHz>" or similar; grab last number
		fields := strings.Fields(sc.Text())
		if len(fields)==0 { continue }
		s := fields[len(fields)-1]
		if m := re.FindStringSubmatch(s); len(m)>=2 {
			v := parseFloat(m[1]); if v>0 { Hz = append(Hz, v) }
		}
	}
	if len(Hz)==0 { return &PitchStats{}, nil }
	sort.Float64s(Hz)
	med := Hz[len(Hz)/2]
	mean := mean(Hz)
	minv := Hz[0]; maxv := Hz[len(Hz)-1]
	midi := hzToMIDI(med)
	note := midiToNoteName(int(math.Round(midi)))
	ps := &PitchStats{
		HzMedian: &med, HzMean: &mean, HzMin: &minv, HzMax: &maxv,
		MIDIMedian: &midi, Note: &note,
	}
	return ps, nil
}

func aubioKey(cfg *Config, in string) (*KeyInfo, error) {
	if err := mustHave(cfg.AubioBin); err != nil { return nil, errors.New("aubio not found") }
	out, _ := runCmd(cfg.AubioBin, "key", "-i", in)
	// typical output like "C major ... confidence 0.54"
	re := regexp.MustCompile(`([A-G][#b]?)[\s\-]+(major|minor|dorian|mixolydian|lydian|phrygian|locrian)?`)
	key, scale := (*string)(nil), (*string)(nil)
	if m := re.FindStringSubmatch(strings.ToLower(out)); len(m)>=2 {
		k := strings.ToUpper(m[1]); key = &k
		if len(m)>=3 && m[2]!="" { s:=m[2]; scale=&s }
	}
	var conf *float64
	reC := regexp.MustCompile(`confidence\s*([0-9]+(\.[0-9]+)?)`)
	if m := reC.FindStringSubmatch(strings.ToLower(out)); len(m)>=2 {
		v := parseFloat(m[1]); conf=&v
	}
	if key==nil && scale==nil && conf==nil { return &KeyInfo{}, nil }
	return &KeyInfo{Key:key, Scale:scale, Conf:conf}, nil
}

// ---------- Math helpers ----------

func mean(xs []float64) float64 {
	if len(xs)==0 { return math.NaN() }
	var s float64; for _,v := range xs { s+=v }
	return s/float64(len(xs))
}
func stddev(xs []float64, m float64) float64 {
	if len(xs)<2 { return 0 }
	var s float64; for _,v := range xs { d:=v-m; s+=d*d }
	return math.Sqrt(s/float64(len(xs)-1))
}
func hzToMIDI(hz float64) float64 { return 69 + 12*math.Log2(hz/440.0) }
func midiToNoteName(m int) string {
	notes := []string{"C","C#","D","D#","E","F","F#","G","G#","A","A#","B"}
	n := (m + 1200) % 12
	oct := (m / 12) - 1
	return fmt.Sprintf("%s%d", notes[n], oct)
}

// ---------- Runner ----------

func analyzeFile(cfg *Config, in string) (*Analysis, error) {
	if _, err := os.Stat(in); err != nil { return nil, err }
	probe, err := ffprobeInfo(cfg, in)
	if err != nil { return nil, err }

	// levels
	peak, rms, _ := ffmpegVolumedetect(cfg, in)
	astatsMap, _ := ffmpegAstatsOverall(cfg, in, cfg.AstatsWin)
	lv := LevelStats{
		PeakDB: peak, RMSDB: rms, CrestDB: peak - rms,
		DCOffset: astatsMap["dc_offset"], ZeroXRate: astatsMap["zero_crossings_rate"],
		NoiseFloor: astatsMap["noise_floor"],
	}
	// clips (if exposed)
	if v, ok := astatsMap["number_of_clipped_samples"]; ok {
		c := int64(v)
		lv.ClipSamples = &c
		if probe.Duration>0 && probe.SampleRate>0 && probe.Channels>0 {
			total := probe.Duration * float64(probe.SampleRate*probe.Channels)
			p := 100.0 * float64(c) / total
			lv.ClipPercent = &p
		}
	}
	// Headroom
	lv.HeadroomDB = 0 - lv.PeakDB

	// LUFS + TruePeak
	var lufs *LUFS
	if cfg.UseEBUR128 {
		if v, err := ffmpegEBUR128(cfg, in); err == nil {
			lufs = &v
			if v.TruePeak != nil {
				lv.TruePeakDBTP = v.TruePeak
			}
		}
	}

	// spectral
	spec, _ := ffmpegSpectral(cfg, in)

	// stereo
	st, _ := ffmpegStereoStuff(cfg, in)

	// bands
	var bands []BandStat
	if cfg.UseBands {
		for _, b := range cfg.Bands {
			if p, r, err := ffmpegBandLoudness(cfg, in, b); err==nil {
				bands = append(bands, BandStat{Band:b, PeakDB:p, RMSDB:r})
			}
		}
	}

	// silence
	sil, _ := detectSilences(cfg, in)
	var silRatio *float64
	if len(sil)>0 && probe.Duration>0 {
		var dur float64
		for _, sp := range sil { dur += sp.End - sp.Start }
		v := (dur / probe.Duration)
		silRatio = &v
	}

	// bpm/onset
	var tempo *TempoStats
	if strings.ToLower(cfg.BPMEngine)=="aubio" {
		if series, err := aubioBPMSeries(cfg, in); err==nil {
			med := series[len(series)/2]
			mu := mean(series)
			sd := stddev(series, mu)
			onr, events, _ := aubioOnsetRate(cfg, in, probe.Duration)
			tempo = &TempoStats{
				BPMMedian:&med, BPMMean:&mu, BPMStd:&sd, Events: events, OnsetPerMin: onr,
			}
		}
	}

	// pitch + key
	ps, _ := aubioPitchStats(cfg, in)
	var key *KeyInfo
	if k, err := aubioKey(cfg, in); err==nil { key = k }

	// hints
	var notes []string
	if lv.ClipSamples!=nil && *lv.ClipSamples>0 {
		notes = append(notes, fmt.Sprintf("Clipping detected: %d samples (%.3f%%)", *lv.ClipSamples, derefFloat(lv.ClipPercent)))
	}
	if lv.TruePeakDBTP!=nil && *lv.TruePeakDBTP> -1.0 {
		notes = append(notes, fmt.Sprintf("True peak dangerously high (%.2f dBTP). Consider -1.5 dBTP ceiling.", *lv.TruePeakDBTP))
	}
	if spec.Flatness!=nil && *spec.Flatness>0.5 {
		notes = append(notes, "High spectral flatness → noise-like content.")
	}
	if st.Correlation!=nil && *st.Correlation<0.2 {
		notes = append(notes, "Low L/R correlation → wide or phasey stereo.")
	}

	return &Analysis{
		File: in, When: time.Now().Format(time.RFC3339),
		Probe: probe, Level: lv, Loudness: lufs, Stereo: st, Spectral: spec,
		Bands: bands, Tempo: tempo, Pitch: ps, Key: key,
		Silence: sil, SilenceRatio: silRatio, Notes: notes,
	}, nil
}

func derefFloat(p *float64) float64 { if p==nil { return math.NaN() }; return *p }

// ---------- Render ----------

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
	if a.Level.TruePeakDBTP!=nil { fmt.Fprintf(&b, " | TruePeak %.2f dBTP", *a.Level.TruePeakDBTP) }
	if a.Level.ClipSamples!=nil && a.Level.ClipPercent!=nil {
		fmt.Fprintf(&b, " | Clips %d (%.3f%%)", *a.Level.ClipSamples, *a.Level.ClipPercent)
	}
	fmt.Fprintf(&b, " | DC %.4f | ZeroX %.2f | NoiseFloor %.2f dBFS\n",
		a.Level.DCOffset, a.Level.ZeroXRate, a.Level.NoiseFloor)
	if a.Loudness != nil {
		fmt.Fprintf(&b, "LUFS: Integrated %.2f LUFS | Range %.2f LU", a.Loudness.Integrated, a.Loudness.Range)
		if a.Loudness.TruePeak!=nil { fmt.Fprintf(&b, " | TruePeak %.2f dBTP", *a.Loudness.TruePeak) }
		fmt.Fprintf(&b, "\n")
	}
	fmt.Fprintf(&b, "Stereo: Mid RMS %.2f dB | Side RMS %.2f dB | Side/Mid %.2f dB",
		a.Stereo.MidRMS, a.Stereo.SideRMS, a.Stereo.SideMidRatioDB)
	if a.Stereo.Correlation!=nil { fmt.Fprintf(&b, " | Corr %.2f", *a.Stereo.Correlation) }
	fmt.Fprintf(&b, "\n")
	if a.Spectral.Centroid!=nil || a.Spectral.Flatness!=nil || a.Spectral.Rolloff95!=nil {
		fmt.Fprintf(&b, "Spectral:")
		if a.Spectral.Centroid!=nil { fmt.Fprintf(&b, " Centroid %.0f Hz", *a.Spectral.Centroid) }
		if a.Spectral.Rolloff95!=nil { fmt.Fprintf(&b, " | Rolloff95 %.0f Hz", *a.Spectral.Rolloff95) }
		if a.Spectral.Flatness!=nil { fmt.Fprintf(&b, " | Flatness %.3f", *a.Spectral.Flatness) }
		if a.Spectral.Spread!=nil { fmt.Fprintf(&b, " | Spread %.3f", *a.Spectral.Spread) }
		if a.Spectral.Skewness!=nil { fmt.Fprintf(&b, " | Skew %.3f", *a.Spectral.Skewness) }
		if a.Spectral.Kurtosis!=nil { fmt.Fprintf(&b, " | Kurt %.3f", *a.Spectral.Kurtosis) }
		fmt.Fprintf(&b, "\n")
	}
	if a.Tempo!=nil {
		fmt.Fprintf(&b, "Tempo: ")
		if a.Tempo.BPMMedian!=nil { fmt.Fprintf(&b, "BPM med %.2f", *a.Tempo.BPMMedian) }
		if a.Tempo.BPMMean!=nil { fmt.Fprintf(&b, " | mean %.2f", *a.Tempo.BPMMean) }
		if a.Tempo.BPMStd!=nil { fmt.Fprintf(&b, " | std %.2f", *a.Tempo.BPMStd) }
		fmt.Fprintf(&b, " | events %d", a.Tempo.Events)
		if a.Tempo.OnsetPerMin!=nil { fmt.Fprintf(&b, " | onsets/min %.2f", *a.Tempo.OnsetPerMin) }
		fmt.Fprintf(&b, "\n")
	}
	if a.Pitch!=nil && (a.Pitch.HzMedian!=nil || a.Pitch.Note!=nil) {
		fmt.Fprintf(&b, "Pitch: ")
		if a.Pitch.HzMedian!=nil { fmt.Fprintf(&b, "median %.2f Hz", *a.Pitch.HzMedian) }
		if a.Pitch.HzMean!=nil { fmt.Fprintf(&b, " | mean %.2f Hz", *a.Pitch.HzMean) }
		if a.Pitch.HzMin!=nil && a.Pitch.HzMax!=nil { fmt.Fprintf(&b, " | min/max %.2f/%.2f Hz", *a.Pitch.HzMin, *a.Pitch.HzMax) }
		if a.Pitch.MIDIMedian!=nil { fmt.Fprintf(&b, " | MIDI %.1f", *a.Pitch.MIDIMedian) }
		if a.Pitch.Note!=nil { fmt.Fprintf(&b, " | note %s", *a.Pitch.Note) }
		fmt.Fprintf(&b, "\n")
	}
	if a.Key!=nil && (a.Key.Key!=nil || a.Key.Scale!=nil) {
		fmt.Fprintf(&b, "Key: ")
		if a.Key.Key!=nil { fmt.Fprintf(&b, "%s", *a.Key.Key) }
		if a.Key.Scale!=nil { fmt.Fprintf(&b, " %s", *a.Key.Scale) }
		if a.Key.Conf!=nil { fmt.Fprintf(&b, " (conf %.2f)", *a.Key.Conf) }
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
		if a.SilenceRatio!=nil {
			fmt.Fprintf(&b, "Silence ratio: %.2f%% of duration\n", *a.SilenceRatio*100)
		}
	}
	if len(a.Notes)>0 {
		fmt.Fprintf(&b, "\nNotes:\n")
		for _, n := range a.Notes { fmt.Fprintf(&b, "  - %s\n", n) }
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
	if a.Level.TruePeakDBTP!=nil { fmt.Fprintf(&b, "- True Peak: `%.2f dBTP`\n", *a.Level.TruePeakDBTP) }
	if a.Level.ClipSamples!=nil && a.Level.ClipPercent!=nil {
		fmt.Fprintf(&b, "- Clipped samples: `%d (%.3f%%)`\n", *a.Level.ClipSamples, *a.Level.ClipPercent)
	}
	fmt.Fprintf(&b, "- DC Offset: `%.4f`\n- Zero-Crossing Rate: `%.2f`\n- Noise Floor: `%.2f dBFS`\n\n",
		a.Level.DCOffset, a.Level.ZeroXRate, a.Level.NoiseFloor)

	if a.Loudness != nil {
		fmt.Fprintf(&b, "## Loudness (EBU R128)\n- Integrated: `%.2f LUFS`\n- Range: `%.2f LU`\n", a.Loudness.Integrated, a.Loudness.Range)
		if a.Loudness.TruePeak!=nil { fmt.Fprintf(&b, "- True Peak: `%.2f dBTP`\n", *a.Loudness.TruePeak) }
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "## Stereo\n- Mid RMS: `%.2f dB`\n- Side RMS: `%.2f dB`\n- Side/Mid: `%.2f dB`\n",
		a.Stereo.MidRMS, a.Stereo.SideRMS, a.Stereo.SideMidRatioDB)
	if a.Stereo.Correlation!=nil { fmt.Fprintf(&b, "- Correlation: `%.2f`\n", *a.Stereo.Correlation) }
	fmt.Fprintf(&b, "\n")

	if a.Spectral.Centroid!=nil || a.Spectral.Rolloff95!=nil || a.Spectral.Flatness!=nil {
		fmt.Fprintf(&b, "## Spectral\n")
		if a.Spectral.Centroid!=nil { fmt.Fprintf(&b, "- Centroid: `%.0f Hz`\n", *a.Spectral.Centroid) }
		if a.Spectral.Rolloff95!=nil { fmt.Fprintf(&b, "- Rolloff (95%%): `%.0f Hz`\n", *a.Spectral.Rolloff95) }
		if a.Spectral.Flatness!=nil { fmt.Fprintf(&b, "- Flatness: `%.3f`\n", *a.Spectral.Flatness) }
		if a.Spectral.Spread!=nil { fmt.Fprintf(&b, "- Spread: `%.3f`\n", *a.Spectral.Spread) }
		if a.Spectral.Skewness!=nil { fmt.Fprintf(&b, "- Skewness: `%.3f`\n", *a.Spectral.Skewness) }
		if a.Spectral.Kurtosis!=nil { fmt.Fprintf(&b, "- Kurtosis: `%.3f`\n", *a.Spectral.Kurtosis) }
		fmt.Fprintf(&b, "\n")
	}

	if a.Tempo!=nil {
		fmt.Fprintf(&b, "## Tempo\n")
		if a.Tempo.BPMMedian!=nil { fmt.Fprintf(&b, "- BPM (median): `%.2f`\n", *a.Tempo.BPMMedian) }
		if a.Tempo.BPMMean!=nil { fmt.Fprintf(&b, "- BPM (mean): `%.2f`\n", *a.Tempo.BPMMean) }
		if a.Tempo.BPMStd!=nil { fmt.Fprintf(&b, "- BPM (stddev): `%.2f`\n", *a.Tempo.BPMStd) }
		fmt.Fprintf(&b, "- Tempo events: `%d`\n", a.Tempo.Events)
		if a.Tempo.OnsetPerMin!=nil { fmt.Fprintf(&b, "- Onsets/min: `%.2f`\n", *a.Tempo.OnsetPerMin) }
		fmt.Fprintf(&b, "\n")
	}

	if a.Pitch!=nil && (a.Pitch.HzMedian!=nil || a.Pitch.Note!=nil) {
		fmt.Fprintf(&b, "## Pitch\n")
		if a.Pitch.HzMedian!=nil { fmt.Fprintf(&b, "- Median: `%.2f Hz`\n", *a.Pitch.HzMedian) }
		if a.Pitch.HzMean!=nil { fmt.Fprintf(&b, "- Mean: `%.2f Hz`\n", *a.Pitch.HzMean) }
		if a.Pitch.HzMin!=nil && a.Pitch.HzMax!=nil { fmt.Fprintf(&b, "- Min/Max: `%.2f / %.2f Hz`\n", *a.Pitch.HzMin, *a.Pitch.HzMax) }
		if a.Pitch.MIDIMedian!=nil { fmt.Fprintf(&b, "- MIDI: `%.1f`\n", *a.Pitch.MIDIMedian) }
		if a.Pitch.Note!=nil { fmt.Fprintf(&b, "- Note: `%s`\n", *a.Pitch.Note) }
		fmt.Fprintf(&b, "\n")
	}

	if a.Key!=nil && (a.Key.Key!=nil || a.Key.Scale!=nil) {
		fmt.Fprintf(&b, "## Key\n")
		if a.Key.Key!=nil { fmt.Fprintf(&b, "- Key: `%s`\n", *a.Key.Key) }
		if a.Key.Scale!=nil { fmt.Fprintf(&b, "- Scale: `%s`\n", *a.Key.Scale) }
		if a.Key.Conf!=nil { fmt.Fprintf(&b, "- Confidence: `%.2f`\n", *a.Key.Conf) }
		fmt.Fprintf(&b, "\n")
	}

	if len(a.Bands)>0 {
		fmt.Fprintf(&b, "## Band Loudness\n\n| Band (Hz) | Peak (dBFS) | RMS (dBFS) |\n|---:|---:|---:|\n")
		for _, bs := range a.Bands {
			fmt.Fprintf(&b, "| %.0f–%.0f | %.2f | %.2f |\n", bs.Band.Lo, bs.Band.Hi, bs.PeakDB, bs.RMSDB)
		}
		fmt.Fprintf(&b, "\n")
	}

	if len(a.Silence)>0 {
		fmt.Fprintf(&b, "## Silence\n")
		for _, s := range a.Silence {
			fmt.Fprintf(&b, "- `%.3f → %.3f` (%.3fs)\n", s.Start, s.End, s.End-s.Start)
		}
		if a.SilenceRatio!=nil {
			fmt.Fprintf(&b, "- Silence ratio: `%.2f%%`\n", *a.SilenceRatio*100)
		}
		fmt.Fprintf(&b, "\n")
	}

	if len(a.Notes)>0 {
		fmt.Fprintf(&b, "## Notes\n")
		for _, n := range a.Notes { fmt.Fprintf(&b, "- %s\n", n) }
		fmt.Fprintf(&b, "\n")
	}
	return b.String()
}

// ---------- Compare ----------

type Diff struct {
	A, B   *Analysis
	Delta  map[string]float64
}

func compare(a, b *Analysis) *Diff {
	d := &Diff{A:a, B:b, Delta: map[string]float64{}}
	d.Delta["peak_db"] = b.Level.PeakDB - a.Level.PeakDB
	d.Delta["rms_db"]  = b.Level.RMSDB  - a.Level.RMSDB
	d.Delta["crest_db"]= b.Level.CrestDB- a.Level.CrestDB
	if a.Loudness!=nil && b.Loudness!=nil {
		d.Delta["lufs_integrated"] = b.Loudness.Integrated - a.Loudness.Integrated
		d.Delta["lufs_range"]      = b.Loudness.Range - a.Loudness.Range
	}
	d.Delta["stereo_side_mid_db"] = b.Stereo.SideMidRatioDB - a.Stereo.SideMidRatioDB
	if a.Tempo!=nil && b.Tempo!=nil && a.Tempo.BPMMedian!=nil && b.Tempo.BPMMedian!=nil {
		d.Delta["bpm_median"] = *b.Tempo.BPMMedian - *a.Tempo.BPMMedian
	}
	d.Delta["duration_s"] = b.Probe.Duration - a.Probe.Duration
	return d
}

func renderDiff(cfg *Config, d *Diff) string {
	switch strings.ToLower(cfg.Report) {
	case "json":
		buf, _ := json.MarshalIndent(d, "", "  ")
		return string(buf)+"\n"
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
		if d.A.Loudness!=nil && d.B.Loudness!=nil {
			row("LUFS (integr.)", d.A.Loudness.Integrated, d.B.Loudness.Integrated, d.Delta["lufs_integrated"], "%.2f")
			row("LUFS Range", d.A.Loudness.Range, d.B.Loudness.Range, d.Delta["lufs_range"], "%.2f")
		}
		row("Side/Mid dB", d.A.Stereo.SideMidRatioDB, d.B.Stereo.SideMidRatioDB, d.Delta["stereo_side_mid_db"], "%.2f")
		if d.A.Tempo!=nil && d.B.Tempo!=nil && d.A.Tempo.BPMMedian!=nil && d.B.Tempo.BPMMedian!=nil {
			row("BPM (median)", *d.A.Tempo.BPMMedian, *d.B.Tempo.BPMMedian, d.Delta["bpm_median"], "%.2f")
		}
		row("Duration (s)", d.A.Probe.Duration, d.B.Probe.Duration, d.Delta["duration_s"], "%.3f")
		return b.String()
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "COMPARE: %s vs %s\n\n", d.A.File, d.B.File)
		for _, k := range []string{"peak_db","rms_db","crest_db","lufs_integrated","lufs_range","stereo_side_mid_db","bpm_median","duration_s"} {
			if v, ok := d.Delta[k]; ok && !math.IsNaN(v) && !math.IsInf(v,0) {
				fmt.Fprintf(&b, "%-20s : %+8.3f\n", k, v)
			}
		}
		return b.String()
	}
}

// ---------- CLI ----------

func main() {
	cfg := defaultConfig()
	outPath := flag.String("o", cfg.OutPath, "output path")
	report  := flag.String("report", cfg.Report, "report: txt|json|md")
	ffmpeg  := flag.String("ffmpeg", cfg.FFmpegBin, "path to ffmpeg")
	ffprobe := flag.String("ffprobe", cfg.FFprobeBin, "path to ffprobe")
	aubio   := flag.String("aubio", cfg.AubioBin, "path to aubio (tempo/key/pitch/onset)")
	bpmEng  := flag.String("bpm-engine", cfg.BPMEngine, "bpm engine: aubio|none")
	bandsStr:= flag.String("bands", "20-60,60-120,120-250,250-500,500-2000,2000-5000,5000-10000,10000-20000", "bands Hz: \"20-60,60-120,...\"")
	noBands := flag.Bool("no-bands", false, "disable band loudness")
	noEbu   := flag.Bool("no-ebur128", false, "disable LUFS ebur128/true peak")
	astWin  := flag.Float64("astats-window", 0.0, "astats window sec (0=overall)")
	silTh   := flag.Float64("silence-threshold", cfg.SilThresDB, "silence threshold dBFS")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "analit — overkill audio analysis\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  analit full <input> [flags]\n  analit compare <inputA> <inputB> [flags]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	args := flag.Args()
	if len(args)<1 { flag.Usage(); os.Exit(2) }

	// apply
	cfg.OutPath=*outPath; cfg.Report=strings.ToLower(*report)
	cfg.FFmpegBin=*ffmpeg; cfg.FFprobeBin=*ffprobe; cfg.AubioBin=*aubio
	cfg.BPMEngine=strings.ToLower(*bpmEng)
	cfg.Bands=parseBands(*bandsStr); cfg.UseBands=!(*noBands)
	cfg.UseEBUR128=!(*noEbu)
	cfg.AstatsWin=*astWin; cfg.SilThresDB=*silTh

	if err := mustHave(cfg.FFmpegBin); err!=nil { fail("ffmpeg not found: %v", err) }
	if err := mustHave(cfg.FFprobeBin); err!=nil { fail("ffprobe not found: %v", err) }
	if cfg.BPMEngine=="aubio" {
		if err := mustHave(cfg.AubioBin); err!=nil {
			fmt.Fprintf(os.Stderr, "[warn] aubio not found; disabling aubio features\n")
			cfg.BPMEngine="none"
		}
	}

	switch strings.ToLower(args[0]) {
	case "full":
		if len(args)<2 { fail("full: missing <input>") }
		in := args[1]
		a, err := analyzeFile(cfg, in)
		if err!=nil { fail("analysis failed: %v", err) }
		if err := writeReport(cfg, a, cfg.OutPath); err!=nil { fail("write: %v", err) }
		fmt.Printf("[+] wrote %s\n", cfg.OutPath)

	case "compare":
		if len(args)<3 { fail("compare: need <inputA> <inputB>") }
		a1, err := analyzeFile(cfg, args[1]); if err!=nil { fail("A: %v", err) }
		a2, err := analyzeFile(cfg, args[2]); if err!=nil { fail("B: %v", err) }
		diff := compare(a1, a2)
		out := renderDiff(cfg, diff)
		if err := os.WriteFile(cfg.OutPath, []byte(out), 0644); err!=nil { fail("write diff: %v", err) }
		fmt.Printf("[+] wrote %s\n", cfg.OutPath)

	default:
		flag.Usage(); os.Exit(2)
	}
}
