package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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
		if !j.ok {
			continue
		}
		if err := ffmpegFilterTo(c, in, j.filter, j.out); err != nil {
			return fmt.Errorf("creating %s failed: %w", j.out, err)
		}
		fmt.Printf("[+] wrote %s\n", j.out)
	}
	return nil
}

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
