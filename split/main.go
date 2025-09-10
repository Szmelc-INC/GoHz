package main

import (
	"flag"
	"os"
)

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
