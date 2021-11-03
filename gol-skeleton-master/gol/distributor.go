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
	//Create a second 2D slice to store next state of the world (odd numbered turns)
	nextWorld := make([][]byte, p.ImageWidth)
	for i := 0; i < p.ImageWidth; i++ {
		nextWorld[i] = make([]byte, p.ImageHeight)
	}

	//TODO read in initial state of GOL using io.go
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
	//fine up to here (works on 0 turns)


	// TODO: Execute all turns of the Game of Life.
	turns := p.Turns
	for turn := 0; turn < turns; turn++ {
		for i := range currentWorld	{
			for j := range currentWorld[i]	{
				if getNumSurroundingCells(i, j, currentWorld) == 3 {
					nextWorld[i][j] = 0xFF
				}	else if getNumSurroundingCells(i, j, currentWorld) < 2	{
					nextWorld[i][j] = 0
				}	else if getNumSurroundingCells(i, j, currentWorld) > 3	{
					nextWorld[i][j] = 0
				}	else if getNumSurroundingCells(i, j, currentWorld) == 2 {
					nextWorld[i][j] = currentWorld[i][j]
				}
			}
		}
		//update current world.
		if turns > 0	{
			for i := range nextWorld	{
				for j := range nextWorld[i]	{
					currentWorld[i][j] = nextWorld[i][j]
					currentWorld[i][j] = nextWorld[i][j]
				}
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


	// TODO: Report the final state using FinalTurnCompleteEvent.
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

//count number of active cells surrounding a current cell
func getNumSurroundingCells(x int, y int, world [][]byte)	int{
	const ALIVE = 0xFF
	var counter = 0
	var succX = x + 1
	var succY = y + 1
	var prevX = x - 1
	var prevY = y - 1
	succX = boundNumber(succX,len(world))
	succY = boundNumber(succY,len(world))
	prevX = boundNumber(prevX,len(world))
	prevY = boundNumber(prevY,len(world))
	if world[prevX][y] == ALIVE	{
		counter++
	}
	if world[prevX][prevY] == ALIVE {
		counter++
	}
	if world[prevX][succY] == ALIVE {
		counter++
	}
	if world[x][succY] == ALIVE {
		counter++
	}
	if world[x][prevY] == ALIVE {
		counter++
	}
	if world[succX][y] == ALIVE {
		counter++
	}
	if world[succX][succY] == ALIVE {
		counter++
	}
	if world[succX][prevY] == ALIVE {
		counter++
	}
	return counter
}

func boundNumber(num int,worldLen int) int{
	if(num < 0){
		return num + worldLen
	}else if(num > worldLen - 1){
		return num - worldLen
	}else{
		return num
	}
}
