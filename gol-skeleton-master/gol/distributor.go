package gol

import (
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
	filename := width + "x" + width + ".pgm"
	c.ioFilename <- filename
	for j, _ := range currentWorld	{
		for k, _ := range currentWorld[j]	{
			currentWorld[j][k] = <- c.ioInput
		}
	}

	turn := 0


	// TODO: Execute all turns of the Game of Life.
	turns := p.Turns
	for turn := turn; turn < turns; turn++ {
		calculateNextState(currentWorld, nextWorld)
		for i := range currentWorld	{
			for j := range currentWorld[i]	{
				currentWorld[i] = nextWorld[j]
			}
		}

	}

	aliveCells := make([]util.Cell, 0, 100)
	for i , _ := range currentWorld {
		for j, _ := range currentWorld[i] {
			if currentWorld[i][j] == 255	{
				var newCell = util.Cell{X: j, Y: i}
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

	c.events <- StateChange{turn, Quitting}
	
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

func calculateNextState(currentWorld [][]byte, newWorld [][]byte) [][]byte {
	//make the cells active according to the rules
	for i , _ := range newWorld	{
		for j, _ := range newWorld[i]	{
			if getNumSurroundingCells(i, j, currentWorld) == 3 {
				newWorld[i][j] = 255
			}	else if getNumSurroundingCells(i, j, currentWorld) < 2	{
				newWorld[i][j] = 0
			}	else if getNumSurroundingCells(i, j, currentWorld) > 3	{
				newWorld[i][j] = 0
			}	else if getNumSurroundingCells(i, j, currentWorld) == 2 {
				newWorld[i][j] = currentWorld[i][j]
			}
		}
	}
	return newWorld
}
//count number of active cells surrounding a current cell
func getNumSurroundingCells(x int, y int, world [][]byte)	int{
	var counter = 0
	var succX = x + 1
	var succY = y + 1
	var prevX = x - 1
	var prevY = y - 1
	if	succX >= len(world)	{
		succX = 0
	}
	if succY >= len(world[x])	{
		succY = 0
	}
	if prevX < 0	{
		prevX = len(world) - 1
	}
	if prevY < 0	{
		prevY = len(world[x]) - 1
	}
	if world[prevX][y] == 255	{
		counter++
	}
	if world[prevX][prevY] == 255 {
		counter++
	}
	if world[prevX][succY] == 255 {
		counter++
	}
	if world[x][succY] == 255 {
		counter++
	}
	if world[x][prevY] == 255 {
		counter++
	}
	if world[succX][y] == 255 {
		counter++
	}
	if world[succX][succY] == 255 {
		counter++
	}
	if world[succX][prevY] == 255 {
		counter++
	}
	return counter

}
