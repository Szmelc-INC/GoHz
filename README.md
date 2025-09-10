# GoHz

GoHz is a fast, trash-polished toolkit for audio analysis and manipulation, written in Go.

## Components

- `split` – splits an audio file into basic stems (bass, drums, music, vocal) using `ffmpeg` filters or the `demucs` command.
- `analize` – command line audio analyzer that reports loudness, spectral stats and more. Requires `ffmpeg` and `ffprobe`, optional `aubio` for tempo/pitch/key.

## Building

Build everything from the project root:

```
./build.sh
```

Or build components individually:

```
go build ./split

go build ./analize
```

## Audio Samples

The `audio/` directory contains test MP3 files for future benchmarking and calibration.

