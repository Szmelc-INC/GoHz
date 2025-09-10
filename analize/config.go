package main

import (
	"strconv"
	"strings"
)

type Bandspec struct{ Lo, Hi float64 }

type Config struct {
	// IO / tools
	OutPath    string
	Report     string // txt|json|md
	FFmpegBin  string
	FFprobeBin string
	AubioBin   string

	// engines
	BPMEngine  string // aubio|none
	UseBands   bool
	Bands      []Bandspec
	UseEBUR128 bool

	// tuning
	AstatsWin  float64
	SilThresDB float64
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
		if part == "" {
			continue
		}
		chunks := strings.Split(part, "-")
		if len(chunks) != 2 {
			continue
		}
		lo, _ := strconv.ParseFloat(strings.TrimSpace(chunks[0]), 64)
		hi, _ := strconv.ParseFloat(strings.TrimSpace(chunks[1]), 64)
		if lo > 0 && hi > lo {
			out = append(out, Bandspec{lo, hi})
		}
	}
	return out
}
