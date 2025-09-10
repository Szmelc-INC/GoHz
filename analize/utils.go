package main

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

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

func parseInt(s string) int       { i, _ := strconv.Atoi(strings.TrimSpace(s)); return i }
func parseInt64(s string) int64   { v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64); return v }
func parseFloat(s string) float64 { f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64); return f }
func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	var s float64
	for _, v := range xs {
		s += v
	}
	return s / float64(len(xs))
}
func stddev(xs []float64, m float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	var s float64
	for _, v := range xs {
		d := v - m
		s += d * d
	}
	return math.Sqrt(s / float64(len(xs)-1))
}
func hzToMIDI(hz float64) float64 { return 69 + 12*math.Log2(hz/440.0) }
func midiToNoteName(m int) string {
	notes := []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	n := (m + 1200) % 12
	oct := (m / 12) - 1
	return fmt.Sprintf("%s%d", notes[n], oct)
}

func derefFloat(p *float64) float64 {
	if p == nil {
		return math.NaN()
	}
	return *p
}
