// main.go
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type cfg struct {
	engine     string
	outFormat  string
	bitrate    string
	ffmpegBin  string
	demucsBin  string

	// stem selection
	stemsCSV  string
	wantBass  bool
	wantDrum  bool
	wantMusic bool
	wantVox   bool

	// preset & gains
	preset       string   // soft|medium|hard
	autoGain     bool
	preGainDB    float64
	gainBassDB   float64
	gainDrumDB   float64
	gainMusicDB  float64
	gainVocalDB  float64

	// cutoff ranges (will be overridden by preset unless user changes)
	// bass
	bassHP   float64
	bassLP   float64
	// drums (kicks)
	drumsHP  float64
	drumsLP  float64
	// music (no kicks)
	musicHP  float64
	musicLP  float64
	// vocals
	vocalHP  float64
	vocalLP  float64
	vocalMid float64
}

func main() {
	c := parseFlags()
	if len(flag.Args()) < 1 {
		fail("no input file provided")
	}
	in := flag.Args()[0]
	if _, err := os.Stat(in); err != nil {
		fail("input not found: %v", err)
	}

	switch c.engine {
	case "demucs":
		if err := runDemucs(c, in); err != nil {
			fail("demucs engine failed: %v", err)
		}
	default:
		if err := mustHave(c.ffmpegBin); err != nil {
			fail("ffmpeg not found in PATH (or via --ffmpeg): %v", err)
		}
		if err := runFfmpegPseudoStems(c, in); err != nil {
			fail("ffmpeg engine failed: %v", err)
		}
	}
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
	if v < 0 { return 0 }
	if v > 1 { return 1 }
	return v
}

// --- Demucs (unchanged) ---
func runDemucs(c *cfg, in string) error {
	if err := mustHave(c.demucsBin); err != nil {
		return fmt.Errorf("demucs not found in PATH (or via --demucs): %w", err)
	}
	cmd := exec.Command(c.demucsBin, "-n", "1", "-o", "demucs_out", in)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	base := baseNoExt(in)
	outRoot := "demucs_out"
	modelDir, err := findSingleChildDir(outRoot)
	if err != nil {
		return fmt.Errorf("demucs output not found: %w", err)
	}
	trackDir, err := findSingleChildDir(filepath.Join(outRoot, modelDir))
	if err != nil {
		return fmt.Errorf("demucs track dir not found: %w", err)
	}
	type m struct{ dem, ours string; ok bool }
	mappings := []m{
		{"bass.wav", base + "-bass." + c.outFormat, c.wantBass},
		{"drums.wav", base + "-drums." + c.outFormat, c.wantDrum},
		{"vocals.wav", base + "-vocal." + c.outFormat, c.wantVox},
		{"other.wav", base + "-music." + c.outFormat, c.wantMusic},
	}
	for _, mm := range mappings {
		if !mm.ok { continue }
		src := filepath.Join(outRoot, modelDir, trackDir, mm.dem)
		if _, statErr := os.Stat(src); statErr != nil {
			fmt.Fprintf(os.Stderr, "[warn] missing demucs stem: %s\n", mm.dem)
			continue
		}
		if err := transcode(c, src, mm.ours); err != nil {
			return fmt.Errorf("transcode %s -> %s: %w", mm.dem, mm.ours, err)
		}
		fmt.Printf("[+] wrote %s\n", mm.ours)
	}
	return nil
}

// --- Heuristic stems via ffmpeg filters ---
func runFfmpegPseudoStems(c *cfg, in string) error {
	base := baseNoExt(in)

	pre := preChain(c)

	type job struct {
		name   string
		filter string
		out    string
		ok     bool
	}

	var jobs []job

	// BASS: tight low band + mild comp + limiter + make-up gain
	if c.wantBass {
		f := chain(pre,
			fmt.Sprintf("highpass=f=%g", c.bassHP),
			fmt.Sprintf("lowpass=f=%g:width_type=h:width=36", c.bassLP),
			"acompressor=threshold=-24dB:ratio=4:attack=8:release=140:makeup=0",
			"alimiter=limit=0.93",
			volumeDB(c.gainBassDB),
		)
		jobs = append(jobs, job{"bass", f, base + "-bass." + c.outFormat, true})
	}

	// DRUMS (KICKS): narrow band for kicks + gate to kill sustained bass + fast comp + limiter + gain
	// agate is great at removing sustained low notes; we hit it before comp.
	if c.wantDrum {
		f := chain(pre,
			fmt.Sprintf("highpass=f=%g", c.drumsHP),
			fmt.Sprintf("lowpass=f=%g", c.drumsLP),
			"agate=threshold=-45dB:ratio=10:attack=3:release=80",
			"acompressor=threshold=-18dB:ratio=6:attack=4:release=80:knee=2",
			"alimiter=limit=0.93",
			volumeDB(c.gainDrumDB),
		)
		jobs = append(jobs, job{"drums", f, base + "-drums." + c.outFormat, true})
	}

	// MUSIC: remove kicks (HP), emphasize side content (stereo), broad band + limiter + gain
	if c.wantMusic {
		f := chain(pre,
			fmt.Sprintf("highpass=f=%g", c.musicHP),
			"stereotools=mlev=0.35:slev=1.10",
			fmt.Sprintf("lowpass=f=%g", c.musicLP),
			"alimiter=limit=0.93",
			volumeDB(c.gainMusicDB),
		)
		jobs = append(jobs, job{"music", f, base + "-music." + c.outFormat, true})
	}

	// VOCALS: center-only bias; band-limit; limiter; gain
	if c.wantVox {
		slev := (1.0 - c.vocalMid) * (-0.25)
		f := chain(pre,
			fmt.Sprintf("stereotools=mlev=%0.3f:slev=%0.3f", c.vocalMid, slev),
			fmt.Sprintf("highpass=f=%g", c.vocalHP),
			fmt.Sprintf("lowpass=f=%g", c.vocalLP),
			"alimiter=limit=0.93",
			volumeDB(c.gainVocalDB),
		)
		jobs = append(jobs, job{"vocal", f, base + "-vocal." + c.outFormat, true})
	}

	for _, j := range jobs {
		if !j.ok { continue }
		if err := ffmpegFilterTo(c, in, j.filter, j.out); err != nil {
			return fmt.Errorf("creating %s failed: %w", j.out, err)
		}
		fmt.Printf("[+] wrote %s\n", j.out)
	}
	return nil
}

// shared pre-chain
func preChain(c *cfg) string {
	var parts []string
	if c.preGainDB != 0 {
		parts = append(parts, fmt.Sprintf("volume=%0.2fdB", c.preGainDB))
	}
	if c.autoGain {
		parts = append(parts, "dynaudnorm=f=250:g=10:n=0")
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ",")
}

func volumeDB(db float64) string {
	if db == 0 {
		return ""
	}
	return fmt.Sprintf("volume=%0.2fdB", db)
}

func chain(filters ...string) string {
	var out []string
	for _, f := range filters {
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	return strings.Join(out, ",")
}

func ffmpegFilterTo(c *cfg, in, filter, out string) error {
	args := []string{"-y", "-i", in, "-vn", "-af", filter}
	switch strings.ToLower(filepath.Ext(out)) {
	case ".mp3":
		args = append(args, "-c:a", "libmp3lame", "-b:a", c.bitrate)
	case ".m4a", ".aac":
		args = append(args, "-c:a", "aac", "-b:a", c.bitrate)
	case ".flac":
		args = append(args, "-c:a", "flac")
	default: // wav
		args = append(args, "-c:a", "pcm_s16le")
	}
	args = append(args, out)
	cmd := exec.Command(c.ffmpegBin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func transcode(c *cfg, in, out string) error {
	args := []string{"-y", "-i", in, "-vn"}
	switch strings.ToLower(filepath.Ext(out)) {
	case ".mp3":
		args = append(args, "-c:a", "libmp3lame", "-b:a", c.bitrate)
	case ".m4a", ".aac":
		args = append(args, "-c:a", "aac", "-b:a", c.bitrate)
	case ".flac":
		args = append(args, "-c:a", "flac")
	default:
		args = append(args, "-c:a", "pcm_s16le")
	}
	args = append(args, out)
	cmd := exec.Command(c.ffmpegBin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func mustHave(bin string) error {
	_, err := exec.LookPath(bin)
	return err
}

func baseNoExt(p string) string {
	dir := filepath.Dir(p)
	name := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	return filepath.Join(dir, name)
}

func findSingleChildDir(root string) (string, error) {
	f, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	var dirs []string
	for _, e := range f {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	if len(dirs) != 1 {
		return "", errors.New("expected exactly one subdir")
	}
	return dirs[0], nil
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "[-] "+format+"\n", a...)
	os.Exit(1)
}
