package main

import (
	"fmt"
	"os"
	"testing"

	"uk.ac.bris.cs/gameoflife/gol"
)

// go test -run ^$ -bench . -benchtime 1x -count 20 | tee result/results.out
// go run golang.org/x/perf/cmd/benchstat -csv result/results.out | tee result/results.csv

func BenchmarkGol(b *testing.B) {
	// Disable all program output apart from benchmark results
	os.Stdout = nil

	for threads := 1; threads <= 16; threads++ {
		b.Run(fmt.Sprintf("%d_workers", threads), func(b *testing.B) {
			traceParams := gol.Params{
				Turns:       100,
				Threads:     threads,
				ImageWidth:  512,
				ImageHeight: 512,
			}
			events := make(chan gol.Event)
			for i := 0; i < b.N; i++ {
				go gol.Run(traceParams, events, nil)
				complete := false
				for !complete {
					event := <-events
					switch event.(type) {
					case gol.FinalTurnComplete:
						complete = true
					}
				}
			}
		})
	}
}
