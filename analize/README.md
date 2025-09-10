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

Compare two files:

```
analize compare original.wav processed.wav -o diff.txt
```

`ffmpeg` and `ffprobe` must be available in `PATH`. Install `aubio` to enable tempo, pitch and key detection.

