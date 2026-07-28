# Audio Duration

This is a fork of https://github.com/hcl/audioduration

A naive go module to get a audio file's duration in second.

## Usage

```go
import "github.com/sydlexius/audioduration"
```

For an audio file (eg. `mp3` format)
```go
f, _ := os.Open("audio.mp3")
defer f.Close()
d, err := audioduration.Mp3(f)
if err != nil {
	// handling error
}
```
Or alternatively
```go
f, _ := os.Open("audio.mp3")
defer f.Close()
d, err := audioduration.Duration(f, audioduration.TypeMp3)
if err != nil {
	// handling error
}
```

## Supported formats

MP3, M4A, MP4, FLAC, DSF, OGG, WAV, AAC, WEBM

## License

The code is licensed under GPLv3.

```
    audioduration - audio duration calculation library in go
    Copyright (C) 2021  hcl

    This program is free software: you can redistribute it and/or modify
    it under the terms of the GNU General Public License as published by
    the Free Software Foundation, either version 3 of the License, or
    (at your option) any later version.

    This program is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
    GNU General Public License for more details.

    You should have received a copy of the GNU General Public License
    along with this program.  If not, see <https://www.gnu.org/licenses/>.
```

## Credits

The audio samples (files under `samples`) for package test are from following source:

* MP3 with tag, M4A, MP4, FLAC, DSF: https://github.com/dhowden/tag/tree/master/testdata
* OGG: https://commons.wikimedia.org/wiki/File:Example.ogg
* MP3(CBR, VBR): https://commons.wikimedia.org/w/index.php?title=File%3ABWV_543-prelude.ogg