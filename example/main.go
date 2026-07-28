package main

import (
	"fmt"
	"log"
	"os"

	"github.com/sydlexius/audioduration"
)

func main() {
	f, err := os.Open("samples/sample.mp3")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			log.Printf("close: %v", cerr)
		}
	}()

	d, err := audioduration.Duration(f, audioduration.TypeMp3)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("duration:", d)
}
