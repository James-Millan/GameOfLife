package gol

import (
	"fmt"
	"strconv"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	//Create a 2D slice to store the world
	currentWorld := make([][]byte, p.ImageWidth)
	for i := 0; i < p.ImageWidth; i++ {
		currentWorld[i] = make([]byte, p.ImageHeight)
	}
	//read in initial state of GOL using io.go
	width := strconv.Itoa(p.ImageWidth)
	filename := width + "x" + width
	fmt.Println(filename)
	c.ioCommand <- ioInput
	c.ioFilename <- filename
	//read file into current world
	for j, _ := range currentWorld	{
		for k, _ := range currentWorld[j]	{
			currentWorld[j][k] = <- c.ioInput
		}
	}

	//initialise channels that are sent to workers.
	workerChannels := make([]chan [][]byte, p.Threads)
	for j := range workerChannels	{
		workerChannels[j] = make(chan [][]byte)
	}


	//Execute all turns of the Game of Life.
	turns := p.Turns
	for turn := 0; turn < turns; turn++ {
		//run the workers and reassemble the slices.
		var nextWorld [][]byte
		var counter = 0
		if p.Threads == 1	{
			nextWorld = processNextState(0, p.ImageWidth - 1, currentWorld, currentWorld)
		} else {
			for i := 0;i < p.Threads;i++ {
				columnsPerChannel := len(currentWorld)/p.Threads
				start := counter
				end := counter + columnsPerChannel
				slice := make([][]byte, columnsPerChannel)
				for i := range slice	{
					slice[i] = make([]byte, p.ImageHeight)
				}
				go worker(start, end, workerChannels[i], slice, currentWorld)
				nextWorld = append(nextWorld, <-workerChannels[i]...)
				counter = end
				}
			}
		fmt.Println(len(nextWorld))
		for i := range nextWorld {
			for j := range nextWorld[i] {
				currentWorld[i][j] = nextWorld[i][j]

			}
		}

	}

	//calculate the alive cells
	aliveCells := make([]util.Cell, 0, p.ImageWidth * p.ImageHeight)
	for i , _ := range currentWorld {
		for j, _ := range currentWorld[i] {
			if currentWorld[i][j] == 0xFF	{
				newCell := util.Cell{X: j, Y: i}
				aliveCells = append(aliveCells, newCell)
			}
		}
	}

	//Report the final state using FinalTurnCompleteEvent.
	c.events <- FinalTurnComplete{
		CompletedTurns: turns,
		Alive: aliveCells}
	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turns, Quitting}
	
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}


//Sends the next state of a slice to the given channel, should be run as goroutine
func processNextState(start, end int, slice [][]byte, world [][]byte ) [][] byte {
	//Making new slice to write changes to
	nextSlice := make([][]byte, len(slice))
	for i := 0; i < len(slice); i++ {
		nextSlice[i] = make([]byte, len(slice[i]))
	}

	for i := range slice	{
		for j := start; j < end; j++	{
			if getNumSurroundingCells(i, j, world) == 3 {
				nextSlice[i][j] = 0xFF
			}	else if getNumSurroundingCells(i, j, world) < 2	{
				nextSlice[i][j] = 0
			}	else if getNumSurroundingCells(i, j, world) > 3	{
				nextSlice[i][j] = 0
			}	else if getNumSurroundingCells(i, j, world) == 2 {
				nextSlice[i][j] = world[i][j]
			}
		}
	}
	fmt.Println(len(nextSlice))
	return nextSlice
}

func worker( start, end int, out chan<- [][]byte, slice [][]byte, world [][]byte)	{
	result := processNextState(start, end, slice, world )
	out <- result
}


//count number of active cells surrounding a current cell
func getNumSurroundingCells(x int, y int, world [][]byte)	int {
	var counter = 0
	var succX = x + 1
	var succY = y + 1
	var prevX = x - 1
	var prevY = y - 1
	if succX >= len(world) {
		succX = 0
	}
	if succY >= len(world[x]) {
		succY = 0
	}
	if prevX < 0 {
		prevX = len(world) - 1
	}
	if prevY < 0 {
		prevY = len(world[x]) - 1
	}
	if world[prevX][y] == 0xFF {
		counter++
	}
	if world[prevX][prevY] == 0xFF {
		counter++
	}
	if world[prevX][succY] == 0xFF {
		counter++
	}
	if world[x][succY] == 0xFF {
		counter++
	}
	if world[x][prevY] == 0xFF {
		counter++
	}
	if world[succX][y] == 0xFF {
		counter++
	}
	if world[succX][succY] == 0xFF {
		counter++
	}
	if world[succX][prevY] == 0xFF {
		counter++
	}
	return counter
}

func boundNumber(num int,worldLen int) int {
	if num < 0 {
		return worldLen - 1
	} else if num > (worldLen - 1) {
		return num - worldLen
	} else {
		return num
	}
}


