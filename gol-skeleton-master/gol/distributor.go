package gol

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
	keyPresses <-chan rune
}

type WorkerNode struct {
	tIn          chan []byte
	tOut         chan []byte
	bIn          chan []byte
	bOut         chan []byte
	sliceChannel chan [][]byte
	requestBoard chan bool
	bSendFirst   bool
	tSendFirst   bool
	turnComplete chan int
}

var initialWorldMutex sync.Mutex

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	//Create a 2D slice to store the world
	currentWorld := make([][]byte, p.ImageWidth)
	for i := 0; i < p.ImageWidth; i++ {
		currentWorld[i] = make([]byte, p.ImageHeight)
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	//read in initial state of GOL using io.go
	width := strconv.Itoa(p.ImageWidth)
	filename := width + "x" + width
	c.ioCommand <- ioInput
	c.ioFilename <- filename
	//read file into current world
	for j, _ := range currentWorld {
		for k, _ := range currentWorld[j] {
			newPixel := <-c.ioInput
			if newPixel == 0xFF {
				c.events <- CellFlipped{Cell: util.Cell{X: j, Y: k}, CompletedTurns: 0}
			}
			currentWorld[j][k] = newPixel
		}
	}

	workerNodes := make([]WorkerNode, p.Threads)
	currentSendsFirst := false
	for i := 0; i < p.Threads; i++ {
		workerNodes[i] = WorkerNode{
			tOut: make(chan []byte),
			bOut: make(chan []byte),
			sliceChannel: make(chan [][]byte),
			requestBoard: make(chan bool),
			turnComplete: make(chan int),
			bSendFirst: currentSendsFirst,
			tSendFirst: currentSendsFirst,
		}
		currentSendsFirst = !currentSendsFirst
	}
	if p.Threads % 2 == 1 {
		workerNodes[len(workerNodes) - 1].bSendFirst = !workerNodes[len(workerNodes) - 1].bSendFirst
	}
	for i := range workerNodes {
		workerNodes[i].bIn = workerNodes[boundNumber(i+1, len(workerNodes))].tOut
		workerNodes[i].tIn = workerNodes[boundNumber(i-1, len(workerNodes))].bOut
	}

	turns := p.Turns
	columnsPerChannel := len(currentWorld) / p.Threads
	//Splitting up world and distributing to channels
	remainderThreads := len(currentWorld) % p.Threads
	offset := 0
	singleWorker := p.Threads == 1
	for sliceNum := 0; sliceNum < p.Threads; sliceNum++ {
		currentSlice := sliceWorld(sliceNum, columnsPerChannel, currentWorld, &remainderThreads, &offset)
		go worker(currentSlice, workerNodes[sliceNum], singleWorker,nil, p.Turns)
	}
	//Reconstructing image from worker channels
	for turn := 0; turn < p.Turns; turn++ {
		var nextWorld [][]byte
		for i := range workerNodes {
			workerNodes[i].requestBoard <- true
			nextSlice := <-workerNodes[i].sliceChannel
			for j := range nextSlice {
				nextWorld = append(nextWorld, nextSlice[j])
			}
			workerNodes[i].turnComplete <- turn
		}
		for i := range currentWorld {
			for j := range currentWorld[i] {
				if currentWorld[i][j] != nextWorld[i][j] {
					c.events <- CellFlipped{Cell: util.Cell{X: i, Y: j}, CompletedTurns: turns}
				}
				currentWorld[i][j] = nextWorld[i][j]
			}
		}
	}

	//calculate the alive cells
	aliveCells := make([]util.Cell, 0, p.ImageWidth*p.ImageHeight)
	for i, _ := range currentWorld {
		for j, _ := range currentWorld[i] {
			if currentWorld[i][j] == 0xFF {
				newCell := util.Cell{X: j, Y: i}
				aliveCells = append(aliveCells, newCell)
			}
		}
	}
	c.events <- FinalTurnComplete{
		CompletedTurns: turns,
		Alive:          aliveCells}
	writeFile(p, c, currentWorld, turns)
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	c.events <- StateChange{turns, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

//Helper function for splitting the world into slices
func sliceWorld(sliceNum int, columnsPerChannel int, currentWorld [][]byte, remainderThreads *int, offset *int) [][]byte {
	currentSlice := [][]byte{}
	//Adding extra column to back of slice to avoid lines of cells that aren't processed
	for i := 0; i < columnsPerChannel; i++ {
		currentSlice = append(currentSlice, currentWorld[boundNumber(sliceNum*columnsPerChannel+i+*offset, len(currentWorld))])
	}
	//Adding extra column to this thread if the world doesn't split into each thread without remainders
	if *remainderThreads > 0 {
		*remainderThreads -= 1
		currentSlice = append(currentSlice,
			currentWorld[boundNumber(sliceNum*columnsPerChannel+columnsPerChannel+*offset, len(currentWorld))])
		*offset += 1
	}
	//Adding extra column to front of slice to avoid lines of cells that aren't processed
	return currentSlice
}

//calculates the number of alive cells given the current world
func getAliveCellsCount(currentWorld [][]byte) int {
	counter := 0
	for i, _ := range currentWorld {
		for j, _ := range currentWorld[i] {
			if currentWorld[i][j] == 0xFF {
				counter++
			}
		}
	}
	return counter
}

//Sends the next state of a slice to the given channel, should be run as goroutine

//the idea here is: append the halos to the slice in the goroutine. then update the middle bit of the slice we need.
//update the halos at the start of each turn. only send back the middle bit when it's requested.
//this will still have a performance enhancement as it's all calculated inside the goroutine rather than main func.
func worker(
	fullSlice [][]byte,
	nodeData WorkerNode,
	onlyWorker bool,
	tickerCall chan int,
	totalTurns int) {
	currentSlice := fullSlice
	//Making new slice to write changes to
	nextSlice := make([][]byte, len(currentSlice))
	for i := range nextSlice {
		nextSlice[i] = make([]byte, len(currentSlice[0]))
	}
	//initialWorldMutex.Unlock()
	var topHalo []byte
	var bottomHalo []byte

	bottomHaloToSend := currentSlice[len(currentSlice)-1]
	topHaloToSend := currentSlice[0]
	if onlyWorker {
		topHalo = bottomHaloToSend
		bottomHalo = topHaloToSend
	}else {
		if nodeData.tSendFirst {
			nodeData.tOut <- topHaloToSend
			bottomHalo = <-nodeData.bIn
		} else {
			bottomHalo = <-nodeData.bIn
			nodeData.tOut <- topHaloToSend
		}
		if nodeData.bSendFirst {
			nodeData.bOut <- bottomHaloToSend
			topHalo = <-nodeData.tIn
		} else {
			topHalo = <-nodeData.tIn
			nodeData.bOut <- bottomHaloToSend
		}
	}
	var topHaloDouble [][]byte
	topHaloDouble = append(topHaloDouble,topHalo)
	currentSlice = append(topHaloDouble, currentSlice...)
	currentSlice = append(currentSlice, bottomHalo)


	for turn := 0; turn < totalTurns; turn++ {
		fmt.Println(turn)
		bottomTurnHaloToSend := currentSlice[len(currentSlice)-2]
		topTurnHaloToSend := currentSlice[1]


		if onlyWorker {
			topHalo = bottomTurnHaloToSend
			bottomHalo = topTurnHaloToSend
		}else {
			if nodeData.tSendFirst {
				nodeData.tOut <- topTurnHaloToSend
				bottomHalo = <-nodeData.bIn
			} else {
				bottomHalo = <-nodeData.bIn
				nodeData.tOut <- topTurnHaloToSend
			}
			if nodeData.bSendFirst {
				nodeData.bOut <- bottomTurnHaloToSend
				topHalo = <-nodeData.tIn
			} else {
				topHalo = <-nodeData.tIn
				nodeData.bOut <- bottomTurnHaloToSend
			}
		}
		for k := 0; k < len(currentSlice[0]); k++	{
			currentSlice[0][k] = topHalo[k]
			currentSlice[len(currentSlice) - 1][k] = bottomHalo[k]
		}
		fmt.Println(len(currentSlice))
		for i := 1; i < len(currentSlice) - 2; i++ {
			for j := range currentSlice[i] {
				surroundingCells := getNumSurroundingCells(i, j, currentSlice)
				if surroundingCells == 3 {
					nextSlice[i-1][j] = 0xFF
				} else if surroundingCells < 2 {
					nextSlice[i-1][j] = 0
				} else if surroundingCells > 3 {
					nextSlice[i-1][j] = 0
				} else if surroundingCells == 2 {
					nextSlice[i-1][j] = currentSlice[i][j]
				}
			}
		}
		select {
			case <- nodeData.requestBoard:
				nodeData.sliceChannel <- nextSlice
				for i := 1; i < len(currentSlice) - 2; i++{
					for j := range currentSlice[i]	{
						currentSlice[i+1][j] = nextSlice[i][j]
					}
				}
			default:
				for i := 1; i < len(currentSlice) - 2; i++{
					for j := range currentSlice[i]	{
						currentSlice[i][j] = nextSlice[i][j]
					}
				}

		}
		turn = <-nodeData.turnComplete
	}
}


//count number of active cells surrounding a current cell
func getNumSurroundingCells(x int, y int, world [][]byte)	int{
	const ALIVE = 0xFF
	var counter = 0
	var succX = x + 1
	var succY = y + 1
	var prevX = x - 1
	var prevY = y - 1
	succX = boundNumber(succX,len(world) - 2)
	succY = boundNumber(succY,len(world[0]))
	prevX = boundNumber(prevX,len(world) - 2)
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
	if num < 0 {
		return num + worldLen
	}else if num > worldLen - 1 {
		return num - worldLen
	}else{
		return num
	}
}

func writeFile(p Params, c distributorChannels, currentWorld [][]byte, turns int) {
	outFile := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(turns)
	c.ioCommand <- ioOutput
	c.ioFilename <- outFile

	//write file bit by bit.
	for i := range currentWorld {
		for j := range currentWorld[i] {
			c.ioOutput <- currentWorld[i][j]
		}
	}
	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- ImageOutputComplete{
		CompletedTurns: turns,
		Filename:       outFile,
	}

}