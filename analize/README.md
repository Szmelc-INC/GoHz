# analize

`analize` (aka `analit`) is a command line audio analyzer built on top of `ffmpeg`/`ffprobe` and optionally `aubio`.

## Build

```
go build ./analize
```

## Usage

Analyze a file:

```
analize full input.wav -o report.txt
```

Split on long silences (e.g., segments separated by ≥1s of silence and trim 0.2s from edges):

```
analize full speech.wav -split-on-silence 1 -trim-ends 0.2
```

Compare two files:

```
analize compare original.wav processed.wav -o diff.txt
```

`ffmpeg` and `ffprobe` must be available in `PATH`. Install `aubio` to enable tempo, pitch and key detection.

