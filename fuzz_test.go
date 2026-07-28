package audioduration

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Every Duration entry point parses UNTRUSTED BINARY INPUT: the consumer hands
// it arbitrary files off a music library, including truncated downloads,
// half-written files, and anything a user dropped in a folder. A parser like
// that has three failure modes worth guarding structurally rather than by
// inspection -- panic (index out of range on a length field), hang (a
// non-advancing cursor in a resync or skip loop), and unbounded allocation.
//
// These are exactly the shapes the #4 review found by hand in walkFrameChain
// and audioEndOffset: a cursor that could fail to advance, a trailer marker
// trusted on a chance payload match. Hand review caught those; a fuzz target
// keeps catching the next one.
//
// A CRASH IS A REAL BUG, AN ERROR IS NOT. Returning an error on garbage is
// correct and expected -- the target asserts only that the parser terminates
// without panicking. It deliberately does NOT assert anything about the
// duration value, because for a malformed stream there is no right answer to
// compare against.

// fuzzTypes maps each seed file extension to the type constant that parses it.
var fuzzTypes = map[string]int{
	".mp3": TypeMp3, ".flac": TypeFlac, ".ogg": TypeOgg,
	".m4a": TypeMp4, ".mp4": TypeMp4, ".dsf": TypeDsd,
	".aac": TypeAac, ".wav": TypeWav,
}

// addSeedCorpus seeds the fuzzer from samples/, so mutation starts at real
// encoder output rather than from random bytes. Reaching a deep code path (a
// valid frame header, then a trailer) by chance is vanishingly unlikely;
// mutating a file that already gets there is not.
func addSeedCorpus(f *testing.F) {
	f.Helper()
	matches, _ := filepath.Glob(filepath.Join("samples", "*"))
	for _, path := range matches {
		ft, ok := fuzzTypes[filepath.Ext(path)]
		if !ok {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		f.Add(data, ft)
	}
	// Hand-built seeds for structures no sample file carries: a bare frame
	// header, an ID3v2 header, and each trailer marker the peel logic trusts.
	f.Add([]byte("\xff\xfb\x90\x00"), TypeMp3)
	f.Add([]byte("ID3\x04\x00\x00\x00\x00\x00\x0a\xff\xfb\x90\x00"), TypeMp3)
	f.Add([]byte("\xff\xfb\x90\x00TAG"), TypeMp3)
	f.Add([]byte("\xff\xfb\x90\x00APETAGEX"), TypeMp3)
	f.Add([]byte("\xff\xfb\x90\x00LYRICS200"), TypeMp3)
	f.Add([]byte{}, TypeMp3)
}

func FuzzDuration(f *testing.F) {
	addSeedCorpus(f)

	f.Fuzz(func(t *testing.T, data []byte, filetype int) {
		// Keep the fuzzer inside the real type domain; an unknown constant just
		// returns an error and burns executions.
		known := false
		for _, ft := range fuzzTypes {
			if ft == filetype {
				known = true
				break
			}
		}
		if !known {
			t.Skip()
		}

		// A HANG is a bug too, and an unbounded loop would otherwise stall the
		// fuzzer rather than report. Run in a goroutine with a deadline so a
		// non-advancing cursor surfaces as a failure instead of a timeout.
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic parsing %d bytes as type %d: %v", len(data), filetype, r)
				}
			}()
			// The error is intentionally discarded: on malformed input it is the
			// CORRECT outcome. Only termination is under test.
			_, _ = Duration(bytes.NewReader(data), filetype)
		}()

		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("hang: parsing %d bytes as type %d exceeded 10s", len(data), filetype)
		}
	})
}
