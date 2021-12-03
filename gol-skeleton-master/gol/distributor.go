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
	tIn                chan []byte
	tOut               chan []byte
	bIn                chan []byte
	bOut               chan []byte
	sliceChannel       chan [][]byte
	bSendFirst         bool
	tSendFirst         bool
	turnChannel        chan int
	cellsChannel       chan int
	synchroniseChannel chan int
	pgmChannel         chan [][]byte
	pauseChannel       chan bool
	killChannel        chan bool
}

var sliceMutex sync.Mutex

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	sliceMutex = sync.Mutex{}
	//Create a 2D slice to store the world
	currentWorld := make([][]byte, p.ImageWidth)
	for i := 0; i < p.ImageWidth; i++ {
		currentWorld[i] = make([]byte, p.ImageHeight)
	}

	//killChannel := make(chan bool)

	//TODO read in initial state of GOL using io.go
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

	//Initialising channels for workers to communicate via
	workerNodes := make([]WorkerNode, p.Threads)
	currentSendsFirst := false
	for i := 0; i < p.Threads; i++ {
		workerNodes[i] = WorkerNode{
			tOut:               make(chan []byte),
			bOut:               make(chan []byte),
			bSendFirst:         currentSendsFirst,
			tSendFirst:         currentSendsFirst,
			sliceChannel:       make(chan [][]byte),
			turnChannel:        make(chan int),
			synchroniseChannel: make(chan int),
			cellsChannel:       make(chan int),
			pgmChannel:         make(chan [][]byte),
			pauseChannel:       make(chan bool),
			killChannel:        make(chan bool),
		}
		currentSendsFirst = !currentSendsFirst
	}
	if p.Threads%2 != 0 {
		workerNodes[len(workerNodes)-1].bSendFirst = !workerNodes[len(workerNodes)-1].bSendFirst
	}
	for i := range workerNodes {
		workerNodes[i].bIn = workerNodes[boundNumber(i+1, len(workerNodes))].tOut
		workerNodes[i].tIn = workerNodes[boundNumber(i-1, len(workerNodes))].bOut
	}
	ticker := time.NewTicker(2 * time.Second)
	turns := p.Turns
	columnsPerChannel := len(currentWorld) / p.Threads
	//Splitting up world and distributing to channels
	remainderThreads := len(currentWorld) % p.Threads
	offset := 0
	singleWorker := p.Threads == 1
	cellFlipOffset := 0
	for sliceNum := 0; sliceNum < p.Threads; sliceNum++ {
		currentSlice := sliceWorld(sliceNum, columnsPerChannel, currentWorld, &remainderThreads, &offset)
		go worker(currentSlice, workerNodes[sliceNum], singleWorker, turns, c.events, cellFlipOffset)
		cellFlipOffset += len(currentSlice)
	}

	//Sending events to workers
	breakLoop := false
	for turn := 0; turn < p.Turns; turn++ {
		syncedTurn := 0
		for w := range workerNodes {
			syncedTurn = <-workerNodes[w].synchroniseChannel
		}
		c.events <- TurnComplete{syncedTurn}
		select {
		case <-ticker.C:
			aliveCells := 0
			for i := range workerNodes {
				workerNodes[i].cellsChannel <- 0
				aliveCells += <-workerNodes[i].cellsChannel
			}
			c.events <- AliveCellsCount{CellsCount: aliveCells, CompletedTurns: syncedTurn}
		case key := <-c.keyPresses:
			switch key {
			case 's':
				imageToWrite := [][]byte{}
				for i := range workerNodes {
					workerNodes[i].pgmChannel <- nil
					imageToWrite = append(imageToWrite, <-workerNodes[i].pgmChannel...)
				}
				writeFile(p, c, imageToWrite, syncedTurn)
			case 'p':
				fmt.Println(syncedTurn)
				for i := range workerNodes {
					workerNodes[i].pauseChannel <- true
				}
				for {
					resumeKey := <-c.keyPresses
					if resumeKey == 'p' {
						fmt.Println("continuing")
						break
					}
				}
				for i := range workerNodes {
					workerNodes[i].pauseChannel <- true
				}
			case 'q':
				breakLoop = true
				imageToWrite := [][]byte{}
				for i := range workerNodes {
					workerNodes[i].killChannel <- true
					imageToWrite = append(imageToWrite, <-workerNodes[i].pgmChannel...)
				}
				writeAndQuit(p, c, imageToWrite, syncedTurn)
			}

		default:
			for i := range workerNodes {
				workerNodes[i].synchroniseChannel <- 0
			}
		}
		if breakLoop {
			break
		}
	}
	//Reconstructing final image
	nextWorld := [][]byte{}
	for i := range workerNodes {
		nextSlice := <-workerNodes[i].sliceChannel
		for j := range nextSlice {
			nextWorld = append(nextWorld, nextSlice[j])
		}
	}
	currentWorld = nextWorld

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

//Sends message to workers to send their slice as a pgm and assembles them
func getPgmFromWorkers(workerNodes []WorkerNode) [][]byte {
	imageToWrite := [][]byte{}
	for i := range workerNodes {
		workerNodes[i].pgmChannel <- nil
		imageToWrite = append(imageToWrite, <-workerNodes[i].pgmChannel...)
	}
	return imageToWrite
}

//Helper function for splitting the world into slices
func sliceWorld(sliceNum int, columnsPerChannel int, currentWorld [][]byte, remainderThreads *int, offset *int) [][]byte {
	currentSlice := [][]byte{}
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
func worker(
	fullSlice [][]byte,
	nodeData WorkerNode,
	onlyWorker bool,
	totalTurns int,
	eventsChannel chan<- Event,
	cellFlipOffset int) {

	currentSlice := fullSlice

	for turn := 0; turn < totalTurns; turn++ {
		bottomHaloToSend := currentSlice[len(currentSlice)-1]
		topHaloToSend := currentSlice[0]
		var topHalo []byte
		var bottomHalo []byte

		if onlyWorker {
			topHalo = bottomHaloToSend
			bottomHalo = topHaloToSend
		} else {
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

		nextSlice := make([][]byte, len(currentSlice))
		for i := range nextSlice {
			nextSlice[i] = make([]byte, len(currentSlice[0]))
		}
		for i := range currentSlice {
			for j := range currentSlice[i] {
				surroundingCells := getNumSurroundingCells(i, j, currentSlice, topHalo, bottomHalo)
				if surroundingCells == 3 {
					nextSlice[i][j] = 0xFF
				} else if surroundingCells < 2 {
					nextSlice[i][j] = 0
				} else if surroundingCells > 3 {
					nextSlice[i][j] = 0
				} else if surroundingCells == 2 {
					nextSlice[i][j] = currentSlice[i][j]
				}
				if nextSlice[i][j] != currentSlice[i][j] {
					eventsChannel <- CellFlipped{Cell: util.Cell{X: i + cellFlipOffset, Y: j}, CompletedTurns: turn}
				}
			}
		}
		nodeData.synchroniseChannel <- turn
		breakingLoop := false
		select {
		case <-nodeData.cellsChannel:
			nodeData.cellsChannel <- getAliveCellsCount(currentSlice)
		case <-nodeData.pgmChannel:
			nodeData.pgmChannel <- currentSlice
		case <-nodeData.pauseChannel:
			<-nodeData.pauseChannel
		case <-nodeData.killChannel:
			breakingLoop = true
			nodeData.pgmChannel <- currentSlice
		case <-nodeData.synchroniseChannel:

		}
		if breakingLoop {
			break
		}
		currentSlice = nextSlice
	}
	nodeData.sliceChannel <- currentSlice
}

//Retrieves specified cell from world, from halos if necessary
func getCellWithHalos(i int, j int, fullSlice [][]byte, topHalo []byte, bottomHalo []byte) byte {
	boundedI := boundNumber(i, len(fullSlice))
	boundedJ := boundNumber(j, len(fullSlice[boundedI]))
	if i < 0 {
		return topHalo[boundedJ]
	} else if i >= len(fullSlice) {
		return bottomHalo[boundedJ]
	} else {
		return fullSlice[boundedI][boundedJ]
	}
}

//count number of active cells surrounding a current cell
func getNumSurroundingCells(x int, y int, fullSlice [][]byte, topHalo []byte, bottomHalo []byte) int {
	const ALIVE = 0xFF
	var counter = 0
	var succX = x + 1
	var succY = y + 1
	var prevX = x - 1
	var prevY = y - 1
	if getCellWithHalos(prevX, y, fullSlice, topHalo, bottomHalo) == ALIVE {
		counter++
	}
	if getCellWithHalos(prevX, prevY, fullSlice, topHalo, bottomHalo) == ALIVE {
		counter++
	}
	if getCellWithHalos(prevX, succY, fullSlice, topHalo, bottomHalo) == ALIVE {
		counter++
	}
	if getCellWithHalos(x, succY, fullSlice, topHalo, bottomHalo) == ALIVE {
		counter++
	}
	if getCellWithHalos(x, prevY, fullSlice, topHalo, bottomHalo) == ALIVE {
		counter++
	}
	if getCellWithHalos(succX, y, fullSlice, topHalo, bottomHalo) == ALIVE {
		counter++
	}
	if getCellWithHalos(succX, succY, fullSlice, topHalo, bottomHalo) == ALIVE {
		counter++
	}
	if getCellWithHalos(succX, prevY, fullSlice, topHalo, bottomHalo) == ALIVE {
		counter++
	}
	return counter
}

func boundNumber(num int, worldLen int) int {
	if num < 0 {
		return num + worldLen
	} else if num > worldLen-1 {
		return num - worldLen
	} else {
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

func writeAndQuit(p Params, c distributorChannels, currentWorld [][]byte, turns int) {
	writeFile(p, c, currentWorld, turns)
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	c.events <- StateChange{turns, Quitting}
}
