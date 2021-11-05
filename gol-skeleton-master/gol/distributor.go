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

	//TODO read in initial state of GOL using io.go
	width := strconv.Itoa(p.ImageWidth)
	filename := width + "x" + width
	c.ioCommand <- ioInput
	c.ioFilename <- filename
	//read file into current world
	for j, _ := range currentWorld	{
		for k, _ := range currentWorld[j]	{
			currentWorld[j][k] = <- c.ioInput
		}
	}

	workerChannels := []chan [][]byte{}
	for i := 0;i < p.Threads;i++ {
		workerChannels = append(workerChannels, make(chan [][]byte))
	}

	// TODO: Execute all turns of the Game of Life.
	turns := p.Turns
	columnsPerChannel := len(currentWorld) / p.Threads
	for turn := 0; turn < turns; turn++ {
		nextWorld := [][]byte{}
		//Splitting up world and distributing to channels
		remainderThreads := len(currentWorld) % p.Threads
		offset := 0
		for sliceNum := 0; sliceNum < p.Threads; sliceNum++{
			go processNewSlice(workerChannels[sliceNum])
			currentSlice := sliceWorld(sliceNum,columnsPerChannel,currentWorld,&remainderThreads,&offset)
			workerChannels[sliceNum] <- currentSlice
		}
		//Reconstructing image from worker channels
		for i := range workerChannels{
			nextSlice := <- workerChannels[i]
			for j := range nextSlice{
				nextWorld = append(nextWorld, nextSlice[j])
			}
		}
		//update current world.
		if turns > 0	{
			for i := range nextWorld	{
				for j := range nextWorld[i]	{
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

//Helper function for splitting the world into slices
func sliceWorld(sliceNum int,columnsPerChannel int,currentWorld [][]byte,remainderThreads *int,offset *int) [][]byte{
	currentSlice := [][]byte{}
	//Adding extra column to back of slice to avoid lines of cells that aren't processed
	extraBackColumnIndex := boundNumber(sliceNum * columnsPerChannel - 1 + *offset,len(currentWorld))
	currentSlice = append(currentSlice,currentWorld[extraBackColumnIndex])
	for i := 0;i < columnsPerChannel;i++{
		currentSlice = append(currentSlice,currentWorld[sliceNum * columnsPerChannel + i + *offset])
	}
	//Adding extra column to this thread if the world doesn't split into each thread without remainders
	if *remainderThreads > 0 {
		*remainderThreads -= 1
		currentSlice = append(currentSlice,
			currentWorld[boundNumber(sliceNum * columnsPerChannel + columnsPerChannel + *offset,len(currentWorld))])
		*offset += 1
	}
	//Adding extra column to front of slice to avoid lines of cells that aren't processed
	extraFrontColumnIndex := boundNumber(sliceNum * columnsPerChannel + columnsPerChannel + *offset,len(currentWorld))
	currentSlice = append(currentSlice,currentWorld[extraFrontColumnIndex])
	return currentSlice
}

//Sends the next state of a slice to the given channel, should be run as goroutine
func processNewSlice(channel chan [][]byte) {
	currentSlice := <- channel
	//Making new slice to write changes to
	nextSlice := make([][]byte, len(currentSlice) - 2)
	for i := range nextSlice{
		nextSlice[i] = make([]byte, len(currentSlice[0]))
	}
	for i := 1;i < len(currentSlice) - 1;i++	{
		for j := range currentSlice[i]	{
			if getNumSurroundingCells(i, j, currentSlice) == 3 {
				nextSlice[i-1][j] = 0xFF
			}	else if getNumSurroundingCells(i, j, currentSlice) < 2	{
				nextSlice[i-1][j] = 0
			}	else if getNumSurroundingCells(i, j, currentSlice) > 3	{
				nextSlice[i-1][j] = 0
			}	else if getNumSurroundingCells(i, j, currentSlice) == 2 {
				nextSlice[i-1][j] = currentSlice[i][j]
			}
		}
	}
	channel <- nextSlice
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
	succY = boundNumber(succY,len(world[0]))
	prevX = boundNumber(prevX,len(world))
	prevY = boundNumber(prevY,len(world[0]))
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
