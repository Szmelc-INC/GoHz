package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	cfg := defaultConfig()
	outPath := flag.String("o", cfg.OutPath, "output path")
	report := flag.String("report", cfg.Report, "report: txt|json|md")
	ffmpeg := flag.String("ffmpeg", cfg.FFmpegBin, "path to ffmpeg")
	ffprobe := flag.String("ffprobe", cfg.FFprobeBin, "path to ffprobe")
	aubio := flag.String("aubio", cfg.AubioBin, "path to aubio (tempo/key/pitch/onset)")
	bpmEng := flag.String("bpm-engine", cfg.BPMEngine, "bpm engine: aubio|none")
	bandsStr := flag.String("bands", "20-60,60-120,120-250,250-500,500-2000,2000-5000,5000-10000,10000-20000", "bands Hz: \"20-60,60-120,...\"")
	noBands := flag.Bool("no-bands", false, "disable band loudness")
	noEbu := flag.Bool("no-ebur128", false, "disable LUFS ebur128/true peak")
	astWin := flag.Float64("astats-window", 0.0, "astats window sec (0=overall)")
	silTh := flag.Float64("silence-threshold", cfg.SilThresDB, "silence threshold dBFS")
	splitSec := flag.Float64("split-on-silence", 0.0, "split input on silence >= seconds (0=off)")
	trimSec := flag.Float64("trim-ends", 0.0, "trim this many seconds from start/end of segments")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "analit — overkill audio analysis\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  analit full <input> [flags]\n  analit compare <inputA> <inputB> [flags]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
		os.Exit(2)
	}

	cfg.OutPath = *outPath
	cfg.Report = strings.ToLower(*report)
	cfg.FFmpegBin = *ffmpeg
	cfg.FFprobeBin = *ffprobe
	cfg.AubioBin = *aubio
	cfg.BPMEngine = strings.ToLower(*bpmEng)
	cfg.Bands = parseBands(*bandsStr)
	cfg.UseBands = !(*noBands)
	cfg.UseEBUR128 = !(*noEbu)
	cfg.AstatsWin = *astWin
	cfg.SilThresDB = *silTh

	if err := mustHave(cfg.FFmpegBin); err != nil {
		fail("ffmpeg not found: %v", err)
	}
	if err := mustHave(cfg.FFprobeBin); err != nil {
		fail("ffprobe not found: %v", err)
	}
	if cfg.BPMEngine == "aubio" {
		if err := mustHave(cfg.AubioBin); err != nil {
			fmt.Fprintf(os.Stderr, "[warn] aubio not found; disabling aubio features\n")
			cfg.BPMEngine = "none"
		}
	}

	switch strings.ToLower(args[0]) {
	case "full":
		if len(args) < 2 {
			fail("full: missing <input>")
		}
		in := args[1]
		a, err := analyzeFile(cfg, in)
		if err != nil {
			fail("analysis failed: %v", err)
		}
		if err := writeReport(cfg, a, cfg.OutPath); err != nil {
			fail("write: %v", err)
		}
		fmt.Printf("[+] wrote %s\n", cfg.OutPath)
		if *splitSec > 0 {
			if _, err := splitBySilence(cfg, in, a, *splitSec, *trimSec); err != nil {
				fail("split: %v", err)
			}
		}

	case "compare":
		if len(args) < 3 {
			fail("compare: need <inputA> <inputB>")
		}
		a1, err := analyzeFile(cfg, args[1])
		if err != nil {
			fail("A: %v", err)
		}
		a2, err := analyzeFile(cfg, args[2])
		if err != nil {
			fail("B: %v", err)
		}
		diff := compare(a1, a2)
		out := renderDiff(cfg, diff)
		if err := os.WriteFile(cfg.OutPath, []byte(out), 0644); err != nil {
			fail("write diff: %v", err)
		}
		fmt.Printf("[+] wrote %s\n", cfg.OutPath)

	default:
		flag.Usage()
		os.Exit(2)
	}
}
