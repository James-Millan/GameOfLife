package main

import (
	"fmt"
	"os"
	"runtime/trace"
	"testing"
	"uk.ac.bris.cs/gameoflife/gol"
	"uk.ac.bris.cs/gameoflife/util"
)

// TestTrace is a special test to be used to generate traces - not a real test
func TestTrace(t *testing.T) {
	traceParams := gol.Params{
		Turns:       10,
		Threads:     4,
		ImageWidth:  64,
		ImageHeight: 64,
	}
	f, _ := os.Create("trace.out")
	events := make(chan gol.Event)
	err := trace.Start(f)
	util.Check(err)
	go gol.Run(traceParams, events, nil)
	for range events {
	}
	trace.Stop()
	err = f.Close()
	util.Check(err)
}

func BenchmarkGameOfLife(b *testing.B) {
	keyPresses := make(chan rune, 10)
	events := make(chan gol.Event, 1000)
	for threads := 1; threads <= 16; threads++	{
		b.Run(fmt.Sprintf("%d_workers", threads), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				params := gol.Params{Turns: 10000, Threads: threads, ImageWidth: 512, ImageHeight: 512}
				gol.Run(params, events, keyPresses)
			}
		})
	}
}
