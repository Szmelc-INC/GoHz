package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

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
	type m struct {
		dem, ours string
		ok        bool
	}
	mappings := []m{
		{"bass.wav", base + "-bass." + c.outFormat, c.wantBass},
		{"drums.wav", base + "-drums." + c.outFormat, c.wantDrum},
		{"vocals.wav", base + "-vocal." + c.outFormat, c.wantVox},
		{"other.wav", base + "-music." + c.outFormat, c.wantMusic},
	}
	for _, mm := range mappings {
		if !mm.ok {
			continue
		}
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
