package main

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
	MidRMS         float64
	SideRMS        float64
	SideMidRatioDB float64
	Correlation    *float64
}

type SpectralStats struct {
	Centroid  *float64 // Hz (proxy)
	Rolloff95 *float64 // Hz
	Flatness  *float64 // 0..1
	Spread    *float64
	Skewness  *float64
	Kurtosis  *float64
}

type TempoStats struct {
	BPMMedian   *float64
	BPMMean     *float64
	BPMStd      *float64
	Events      int
	OnsetPerMin *float64
}

type PitchStats struct {
	HzMedian   *float64
	HzMean     *float64
	HzMin      *float64
	HzMax      *float64
	MIDIMedian *float64
	Note       *string // e.g. "A#3"
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
	File         string
	When         string
	Probe        ProbeInfo
	Level        LevelStats
	Loudness     *LUFS
	Stereo       StereoStats
	Spectral     SpectralStats
	Bands        []BandStat
	Tempo        *TempoStats
	Pitch        *PitchStats
	Key          *KeyInfo
	Silence      []SilenceSpan
	SilenceRatio *float64
	SilenceTotal *float64
	Notes        []string // warnings/suggestions
}

type Diff struct {
	A, B  *Analysis
	Delta map[string]float64
}
