package audioduration

import (
	"fmt"
	"math"
	"os"
	"testing"
)

const delta = 0.0001

type audioTest struct {
	path     string
	duration float64
}

func TestFLAC(t *testing.T) {
	sampleDuration := 3.399365
	testFile := "samples/sample.flac"
	file, err := os.Open(testFile)
	if err != nil {
		t.Errorf("Sample FLAC file(%s): %s.\n", testFile, err)
	}
	defer file.Close()
	d, err := FLAC(file)
	fmt.Println(sampleDuration, d)
	if err != nil {
		t.Errorf("%s\n", err)
	}
	if math.Abs(d-sampleDuration) > delta {
		t.Errorf("too much error, expected '%v', found '%v'\n", sampleDuration, d)
	}
}

func TestMp4(t *testing.T) {
	sampleDuration := 3.413333
	testFile := "samples/sample.mp4"
	file, err := os.Open(testFile)
	if err != nil {
		t.Errorf("Sample MP4 file(%s): %s.\n", testFile, err)
	}
	defer file.Close()
	d, err := Mp4(file)
	fmt.Println(sampleDuration, d)
	if err != nil {
		t.Errorf("%s\n", err)
	}
	if math.Abs(d-sampleDuration) > delta {
		t.Errorf("too much error, expected '%v', found '%v'\n", sampleDuration, d)
	}
}

func TestM4a(t *testing.T) {
	sampleDuration := 3.413333
	testFile := "samples/sample.m4a"
	file, err := os.Open(testFile)
	if err != nil {
		t.Errorf("Sample M4A file(%s): %s.\n", testFile, err)
	}
	defer file.Close()
	d, err := Mp4(file)
	fmt.Println(sampleDuration, d)
	if err != nil {
		t.Errorf("%s\n", err)
	}
	if math.Abs(d-sampleDuration) > delta {
		t.Errorf("too much error, expected '%v', found '%v'\n", sampleDuration, d)
	}
}

func TestMp3FileSet(t *testing.T) {
	testFileSet := map[string]audioTest{
		// https://commons.wikimedia.org/w/index.php?title=File%3ABWV_543-prelude.ogg
		"MPEG Layer 3 (CBR)": {"samples/sample_cbr.mp3", 3.030204},
		// https://commons.wikimedia.org/w/index.php?title=File%3ABWV_543-prelude.ogg
		"MPEG Layer 3 (VBR)": {"samples/sample_vbr.mp3", 3.030204},
		// https://github.com/dhowden/tag/tree/master/testdata
		"MP3 with ID3 tags":    {"samples/sample.id3v24.mp3", 3.448125},
		"MP3 without ID3 tags": {"samples/sample.mp3", 3.448125},
	}
	for k, v := range testFileSet {
		fmt.Printf("Testing: %s\n", k)
		file, err := os.Open(v.path)
		if err != nil {
			t.Errorf("Sample MP3 file(%s): %s.\n", v.path, err)
		}
		defer file.Close()
		d, err := Mp3(file)
		fmt.Println(v.duration, d)
		if err != nil {
			t.Errorf("Sample MP3 file(%s): %s.\n", v.path, err)
		}
		if math.Abs(d-v.duration) > delta {
			t.Errorf("too much error, expected '%v', found '%v' on item '%v'\n", v.duration, d, k)
		}
	}
}

func TestMp2(t *testing.T) {
	t.SkipNow()
	testFile := "samples/sample.mp2"
	file, err := os.Open(testFile)
	if err != nil {
		t.Errorf("Sample MP3 file(%s): %s.\n", testFile, err)
	}
	defer file.Close()
	d, err := Mp3(file)
	if err != nil {
		t.Errorf("Sample MP3 file(%s): %s.\n", testFile, err)
	}
	fmt.Println(d)
}

func TestOgg(t *testing.T) {
	sampleDuration := 6.104036
	// https://commons.wikimedia.org/wiki/File:Example.ogg
	// https://upload.wikimedia.org/wikipedia/commons/c/c8/Example.ogg
	testFile := "samples/example.ogg"
	file, err := os.Open(testFile)
	if err != nil {
		t.Errorf("Sample OGG file(%s): %s.\n", testFile, err)
	}
	defer file.Close()
	d, err := Ogg(file)
	fmt.Println(sampleDuration, d)
	if err != nil {
		t.Errorf("Sample OGG file(%s): %s.\n", testFile, err)
	}
	if math.Abs(d-sampleDuration) > delta {
		t.Errorf("too much error, expected '%v', found '%v'\n", sampleDuration, d)
	}
}

func TestDSD(t *testing.T) {
	// t.SkipNow()
	sampleDuration := 1.4685
	testFile := "samples/sample.dsf"
	file, err := os.Open(testFile)
	if err != nil {
		t.Errorf("Sample DSD file(%s): %s.\n", testFile, err)
	}
	d, err := DSD(file)
	defer file.Close()
	fmt.Println(sampleDuration, d)
	if err != nil {
		t.Errorf("Sample DSD file(%s): %s.\n", testFile, err)
	}
	if math.Abs(d-sampleDuration) > delta {
		t.Errorf("too much error, expected '%v', found '%v'\n", sampleDuration, d)
	}
}

func TestAac(t *testing.T) {
	sampleDuration := 2.020136
	testFile := "samples/sample.aac"
	file, err := os.Open(testFile)
	if err != nil {
		t.Errorf("Sample aac file(%s): %s.\n", testFile, err)
	}
	d, err := AAC(file)
	defer file.Close()
	fmt.Println(sampleDuration, d)
	if err != nil {
		t.Errorf("Sample aac file(%s): %s.\n", testFile, err)
	}
	if math.Abs(d-sampleDuration) > delta {
		t.Errorf("too much error, expected '%v', found '%v'\n", sampleDuration, d)
	}
}

func TestWebM(t *testing.T) {
	sampleDuration := 2.028
	testFile := "samples/sample.webm"
	file, err := os.Open(testFile)
	if err != nil {
		t.Errorf("Sample webm file(%s): %s.\n", testFile, err)
	}
	d, err := WebM(file)
	defer file.Close()
	fmt.Println(sampleDuration, d)
	if err != nil {
		t.Errorf("Sample webm file(%s): %s.\n", testFile, err)
	}
	if math.Abs(d-sampleDuration) > delta {
		t.Errorf("too much error, expected '%v', found '%v'\n", sampleDuration, d)
	}
}
