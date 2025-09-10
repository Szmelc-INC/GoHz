package main

import (
	"flag"
	"strings"
)

type cfg struct {
	engine    string
	outFormat string
	bitrate   string
	ffmpegBin string
	demucsBin string

	// stem selection
	stemsCSV  string
	wantBass  bool
	wantDrum  bool
	wantMusic bool
	wantVox   bool

	// preset & gains
	preset      string // soft|medium|hard
	autoGain    bool
	preGainDB   float64
	gainBassDB  float64
	gainDrumDB  float64
	gainMusicDB float64
	gainVocalDB float64

	// cutoff ranges (will be overridden by preset unless user changes)
	// bass
	bassHP float64
	bassLP float64
	// drums (kicks)
	drumsHP float64
	drumsLP float64
	// music (no kicks)
	musicHP float64
	musicLP float64
	// vocals
	vocalHP  float64
	vocalLP  float64
	vocalMid float64
}

func parseFlags() *cfg {
	c := &cfg{}
	// engine / io
	flag.StringVar(&c.engine, "engine", "ffmpeg", "separation engine: ffmpeg|demucs")
	flag.StringVar(&c.outFormat, "out-format", "wav", "output format/extension (wav|mp3|flac|m4a|...)")
	flag.StringVar(&c.bitrate, "bitrate", "320k", "bitrate for lossy formats (mp3/aac)")
	flag.StringVar(&c.ffmpegBin, "ffmpeg", "ffmpeg", "path to ffmpeg")
	flag.StringVar(&c.demucsBin, "demucs", "demucs", "path to demucs")

	// stem selection
	flag.StringVar(&c.stemsCSV, "stems", "bass,drums,music,vocal", "comma list: bass,drums,music,vocal")

	// preset & gains
	flag.StringVar(&c.preset, "preset", "hard", "split preset: soft|medium|hard")
	flag.BoolVar(&c.autoGain, "auto-gain", true, "light dynamic normalization before splitting")
	flag.Float64Var(&c.preGainDB, "pregain-db", -4.0, "pre volume pad (dB) to avoid clipping")
	flag.Float64Var(&c.gainBassDB, "gain-bass", 5.0, "post-gain for bass stem (dB)")
	flag.Float64Var(&c.gainDrumDB, "gain-drums", 6.0, "post-gain for drums stem (dB)")
	flag.Float64Var(&c.gainMusicDB, "gain-music", 4.0, "post-gain for music stem (dB)")
	flag.Float64Var(&c.gainVocalDB, "gain-vocal", 4.0, "post-gain for vocal stem (dB)")

	// defaults (will be overridden by preset)
	flag.Float64Var(&c.bassHP, "bass-hp", 30, "bass highpass Hz")
	flag.Float64Var(&c.bassLP, "bass-lp", 180, "bass lowpass Hz")
	flag.Float64Var(&c.drumsHP, "drums-hp", 35, "drums highpass Hz (kicks)")
	flag.Float64Var(&c.drumsLP, "drums-lp", 160, "drums lowpass Hz (kicks)")
	flag.Float64Var(&c.musicHP, "music-hp", 180, "music highpass Hz (remove kicks)")
	flag.Float64Var(&c.musicLP, "music-lp", 18000, "music lowpass Hz")
	flag.Float64Var(&c.vocalHP, "vocal-hp", 160, "vocal highpass Hz")
	flag.Float64Var(&c.vocalLP, "vocal-lp", 9000, "vocal lowpass Hz")
	flag.Float64Var(&c.vocalMid, "vocal-mid", 0.95, "0..1 mid (center) level for vocals (stereotools)")

	flag.Parse()

	// normalize stems
	want := map[string]*bool{
		"bass":  &c.wantBass,
		"drums": &c.wantDrum,
		"music": &c.wantMusic,
		"vocal": &c.wantVox,
	}
	for _, s := range strings.Split(c.stemsCSV, ",") {
		s = strings.ToLower(strings.TrimSpace(s))
		if p, ok := want[s]; ok {
			*p = true
		}
	}
	if !c.wantBass && !c.wantDrum && !c.wantMusic && !c.wantVox {
		c.wantBass, c.wantDrum, c.wantMusic, c.wantVox = true, true, true, true
	}

	// preset shaping (unless user overrides via flags after; these are just defaults we already set)
	switch strings.ToLower(c.preset) {
	case "soft":
		c.bassHP, c.bassLP = 25, 220
		c.drumsHP, c.drumsLP = 30, 200
		c.musicHP = 160
		c.vocalMid = clamp01(c.vocalMid)
	case "medium":
		c.bassHP, c.bassLP = 30, 200
		c.drumsHP, c.drumsLP = 35, 180
		c.musicHP = 180
		c.vocalMid = clamp01(c.vocalMid)
	default: // hard
		c.preset = "hard"
		c.bassHP, c.bassLP = 30, 180
		c.drumsHP, c.drumsLP = 38, 160
		c.musicHP = 190
		c.vocalMid = clamp01(c.vocalMid)
	}

	return c
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
