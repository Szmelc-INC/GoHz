# split

`split` separates an audio file into rough stems using `ffmpeg` filters or the `demucs` command.

## Build

```
go build ./split
```

## Usage

Split into stems with `ffmpeg`:

```
split song.mp3
```

Use `demucs` engine:

```
split -engine demucs song.mp3
```

Outputs bass, drums, music and vocal stems alongside the input file. `ffmpeg` is required; `demucs` must be installed for the demucs engine.

