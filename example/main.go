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
	defer f.Close()

	d, err := audioduration.Duration(f, audioduration.TypeMp3)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("duration:", d)
}
