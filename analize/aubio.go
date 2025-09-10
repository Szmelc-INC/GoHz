package main

import (
	"bufio"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func aubioBPMSeries(cfg *Config, in string) ([]float64, error) {
	if err := mustHave(cfg.AubioBin); err != nil {
		return nil, errors.New("aubio not found")
	}
	out, err := runCmd(cfg.AubioBin, "tempo", "-i", in)
	if err != nil && out == "" {
		return nil, fmt.Errorf("aubio tempo failed: %v", err)
	}
	re := regexp.MustCompile(`([0-9]+(\.[0-9]+)?)\s*bpm`)
	var vals []float64
	sc := bufio.NewScanner(strings.NewReader(strings.ToLower(out)))
	for sc.Scan() {
		if m := re.FindStringSubmatch(sc.Text()); len(m) >= 2 {
			vals = append(vals, parseFloat(m[1]))
		}
	}
	if len(vals) == 0 {
		return nil, fmt.Errorf("no bpm series")
	}
	return vals, nil
}

func aubioOnsetRate(cfg *Config, in string, durSec float64) (*float64, int, error) {
	if err := mustHave(cfg.AubioBin); err != nil {
		return nil, 0, errors.New("aubio not found")
	}
	out, _ := runCmd(cfg.AubioBin, "onset", "-i", in)
	var count int
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if _, err := strconv.ParseFloat(strings.Fields(line)[0], 64); err == nil {
			count++
		}
	}
	if durSec <= 0 {
		return nil, count, nil
	}
	rate := float64(count) / (durSec / 60.0)
	return &rate, count, nil
}

func aubioPitchStats(cfg *Config, in string) (*PitchStats, error) {
	if err := mustHave(cfg.AubioBin); err != nil {
		return nil, errors.New("aubio not found")
	}
	out, _ := runCmd(cfg.AubioBin, "pitch", "-i", in)
	re := regexp.MustCompile(`([0-9]+(\.[0-9]+)?)`)
	var Hz []float64
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 {
			continue
		}
		s := fields[len(fields)-1]
		if m := re.FindStringSubmatch(s); len(m) >= 2 {
			v := parseFloat(m[1])
			if v > 0 {
				Hz = append(Hz, v)
			}
		}
	}
	if len(Hz) == 0 {
		return &PitchStats{}, nil
	}
	sort.Float64s(Hz)
	med := Hz[len(Hz)/2]
	mean := mean(Hz)
	minv := Hz[0]
	maxv := Hz[len(Hz)-1]
	midi := hzToMIDI(med)
	note := midiToNoteName(int(math.Round(midi)))
	ps := &PitchStats{
		HzMedian: &med, HzMean: &mean, HzMin: &minv, HzMax: &maxv,
		MIDIMedian: &midi, Note: &note,
	}
	return ps, nil
}

func aubioKey(cfg *Config, in string) (*KeyInfo, error) {
	if err := mustHave(cfg.AubioBin); err != nil {
		return nil, errors.New("aubio not found")
	}
	out, _ := runCmd(cfg.AubioBin, "key", "-i", in)
	re := regexp.MustCompile(`([A-G][#b]?)\s+(major|minor|dorian|mixolydian|lydian|phrygian|locrian)?`)
	key, scale := (*string)(nil), (*string)(nil)
	if m := re.FindStringSubmatch(strings.ToLower(out)); len(m) >= 2 {
		k := strings.ToUpper(m[1])
		key = &k
		if len(m) >= 3 && m[2] != "" {
			s := m[2]
			scale = &s
		}
	}
	var conf *float64
	reC := regexp.MustCompile(`confidence\s*([0-9]+(\.[0-9]+)?)`)
	if m := reC.FindStringSubmatch(strings.ToLower(out)); len(m) >= 2 {
		v := parseFloat(m[1])
		conf = &v
	}
	if key == nil && scale == nil && conf == nil {
		return &KeyInfo{}, nil
	}
	return &KeyInfo{Key: key, Scale: scale, Conf: conf}, nil
}
