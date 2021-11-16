package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"testing"
	"uk.ac.bris.cs/gameoflife/gol"
	"uk.ac.bris.cs/gameoflife/util"
)

// TestGol tests 16x16, 64x64 and 512x512 images on 0, 1 and 100 turns using 1-16 worker threads.
func TestGol(t *testing.T) {
	tests := []gol.Params{
		{ImageWidth: 16, ImageHeight: 16},
		{ImageWidth: 64, ImageHeight: 64},
		{ImageWidth: 512, ImageHeight: 512},
	}
	for _, p := range tests {
		for _, turns := range []int{0, 1, 100} {
			p.Turns = turns
			expectedAlive := readAliveCells(
				"check/images/"+fmt.Sprintf("%vx%vx%v.pgm", p.ImageWidth, p.ImageHeight, turns),
				p.ImageWidth,
				p.ImageHeight,
			)
			for threads := 1; threads <= 16; threads++ {
				p.Threads = threads
				testName := fmt.Sprintf("%dx%dx%d-%d", p.ImageWidth, p.ImageHeight, p.Turns, p.Threads)
				t.Run(testName, func(t *testing.T) {
					events := make(chan gol.Event)
					go gol.Run(p, events, nil)
					var cells []util.Cell
					for event := range events {
						switch e := event.(type) {
						case gol.FinalTurnComplete:
							cells = e.Alive
						}
					}
					assertEqualBoard(t, cells, expectedAlive, p)
				})
			}
		}
	}
}

func boardFail(t *testing.T, given, expected []util.Cell, p gol.Params) bool {
	errorString := fmt.Sprintf("-----------------\n\n  FAILED TEST\n  %vx%v\n  %d Workers\n  %d Turns\n", p.ImageWidth, p.ImageHeight, p.Threads, p.Turns)
	if p.ImageWidth == 16 && p.ImageHeight == 16 {
		errorString = errorString + util.AliveCellsToString(given, expected, p.ImageWidth, p.ImageHeight)
	}
	t.Error(errorString)
	return false
}

func assertEqualBoard(t *testing.T, given, expected []util.Cell, p gol.Params) bool {
	givenLen := len(given)
	expectedLen := len(expected)

	if givenLen != expectedLen {
		return boardFail(t, given, expected, p)
	}

	visited := make([]bool, expectedLen)
	for i := 0; i < givenLen; i++ {
		element := given[i]
		found := false
		for j := 0; j < expectedLen; j++ {
			if visited[j] {
				continue
			}
			if expected[j] == element {
				visited[j] = true
				found = true
				break
			}
		}
		if !found {
			return boardFail(t, given, expected, p)
		}
	}

	return true
}

func readAliveCells(path string, width, height int) []util.Cell {
	data, ioError := ioutil.ReadFile(path)
	util.Check(ioError)

	fields := strings.Fields(string(data))

	if fields[0] != "P5" {
		panic("Not a pgm file")
	}

	imageWidth, _ := strconv.Atoi(fields[1])
	if imageWidth != width {
		panic("Incorrect width")
	}

	imageHeight, _ := strconv.Atoi(fields[2])
	if imageHeight != height {
		panic("Incorrect height")
	}

	maxval, _ := strconv.Atoi(fields[3])
	if maxval != 255 {
		panic("Incorrect maxval/bit depth")
	}

	image := []byte(fields[4])

	var cells []util.Cell
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			cell := image[0]
			if cell != 0 {
				cells = append(cells, util.Cell{
					X: x,
					Y: y,
				})
			}
			image = image[1:]
		}
	}
	return cells
}
/*
func BenchmarkGol(b *testing.B) {
	os.Stdout = nil
	for threads := 1; threads <= 16; threads++	{
		keyPresses := make(chan rune, 10)
		events := make(chan gol.Event, 1000)
		params := gol.Params{Turns: 100, Threads: threads, ImageWidth: 16, ImageHeight: 16}
		ioCommand := make(chan ioCommand)
		ioIdle := make(chan bool)
		ioFilename := make(chan string)
		ioOutput := make(chan uint8)
		ioInput := make(chan uint8)

		ioChannels := ioChannels{
			command:  ioCommand,
			idle:     ioIdle,
			filename: ioFilename,
			output:   ioOutput,
			input:    ioInput,
		}
		go startIo(p, ioChannels)

		distributorChannels := distributorChannels{
			events:     events,
			ioCommand:  ioCommand,
			ioIdle:     ioIdle,
			ioFilename: ioFilename,
			ioOutput:   ioOutput,
			ioInput:    ioInput,
			keyPresses: keyPresses,
		}

		b.Run(fmt.Sprintf("%d_workers", threads), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				distributor(params, distributorChannels)
			}
		})
		close(keyPresses)
	}
}

 */



