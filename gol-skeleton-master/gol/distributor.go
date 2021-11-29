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
	tickerChannel      chan int
	turnChannel        chan int
	synchroniseChannel chan int
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

	killChannel := make(chan bool)

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

	workerNodes := make([]WorkerNode, p.Threads)
	currentSendsFirst := false
	for i := 0; i < p.Threads; i++ {
		workerNodes[i] = WorkerNode{
			tOut:               make(chan []byte),
			bOut:               make(chan []byte),
			bSendFirst:         currentSendsFirst,
			tSendFirst:         currentSendsFirst,
			sliceChannel:       make(chan [][]byte),
			tickerChannel:      make(chan int),
			turnChannel:        make(chan int),
			synchroniseChannel: make(chan int),
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

	go sendTickerCalls(killChannel, workerNodes, c.events)
	//TODO implement keypress logic.

	// Execute all turns of the Game of Life.
	//turnCounter := 0

	/*go func() {
		for  {
			select {
			case command := <-c.keyPresses:
				switch command	{
				case 'p':
					fmt.Println("p")
				case 's':
					fmt.Println("s")
					writeFile(p, c, currentWorld, turnCounter)
				case 'q':
					fmt.Println("q")
					writeAndQuit(p, c, currentWorld, turnCounter)
				}
			default:
			}
		}
	}()
	*/
	turns := p.Turns
	columnsPerChannel := len(currentWorld) / p.Threads
	//Splitting up world and distributing to channels
	remainderThreads := len(currentWorld) % p.Threads
	offset := 0
	singleWorker := p.Threads == 1
	for sliceNum := 0; sliceNum < p.Threads; sliceNum++ {
		currentSlice := sliceWorld(sliceNum, columnsPerChannel, currentWorld, &remainderThreads, &offset)
		go worker(currentSlice, workerNodes[sliceNum], singleWorker, nil, nil, turns, c.events)
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

	/*select {
	case <-ticker.C:
		cells := getAliveCellsCount(currentWorld)
		c.events <- AliveCellsCount{CellsCount: cells, CompletedTurns: turnCounter}
	default:
	}
	select {
	case command := <-c.keyPresses:
		switch command {
		case 'p':
			fmt.Println("p")
			for {
				unPause := <-c.keyPresses
				done := false
				switch unPause {
				case 'p':
					done = true
					break
				case 's':
					writeFile(p, c, currentWorld, turnCounter)
				case 'q':
					writeFile(p, c, currentWorld, turnCounter)
					done = true
					turn = p.Turns
				}
				if done {
					break
				}
			}
		case 's':
			fmt.Println("s")
			writeFile(p, c, currentWorld, turnCounter)
		case 'q':
			fmt.Println("q")
			writeFile(p, c, currentWorld, turnCounter)
			turn = p.Turns
		}
	default:
	}
	//update current world.
	turnCounter++
	c.events <- TurnComplete{CompletedTurns: turnCounter}
	*/

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

func sendTickerCalls(killChannel chan bool, workerNodes []WorkerNode, eventsChannel chan<- Event) {
	ticker := time.NewTicker(2 * time.Second)

	for {
		select {
		case <-ticker.C:
			aliveCells := 0
			var turn int
			for i := range workerNodes {
				fmt.Println(turn)
				workerNodes[i].tickerChannel <- 0
				aliveCells += <-workerNodes[i].tickerChannel
				turn = <-workerNodes[i].turnChannel
			}
			eventsChannel <- AliveCellsCount{CellsCount: aliveCells, CompletedTurns: turn}
		default:
		}
	}
}

//Sends the next state of a slice to the given channel, should be run as goroutine
func worker(
	fullSlice [][]byte,
	nodeData WorkerNode,
	onlyWorker bool,
	turnCompleted chan int,
	requestBoard chan bool,
	totalTurns int,
	eventsChannel chan<- Event) {

	currentSlice := fullSlice
	synchronising := false
	synchronisedTurn := 0

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

		select {
		case <-nodeData.synchroniseChannel:
			nodeData.synchroniseChannel <- turn
			synchronisedTurn = <-nodeData.synchroniseChannel
			synchronising = true
		default:
		}

		if synchronising && turn == synchronisedTurn {
			nodeData.synchroniseChannel <- 0
			<-nodeData.synchroniseChannel
			synchronising = false
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
					eventsChannel <- CellFlipped{Cell: util.Cell{X: i, Y: j}, CompletedTurns: turn}
				}
			}
		}
		select {
		case <-nodeData.tickerChannel:
			nodeData.tickerChannel <- getAliveCellsCount(currentSlice)
			nodeData.turnChannel <- turn
		default:
		}
		currentSlice = nextSlice
	}
	nodeData.sliceChannel <- currentSlice
}

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

func synchronise(workerNodes []WorkerNode) {
	//Establishing turn to sync on
	latestTurn := 0
	for i := range workerNodes {
		workerNodes[i].synchroniseChannel <- 0
		t := <-workerNodes[i].synchroniseChannel
		if t > latestTurn {
			latestTurn = t
		}
	}
	//Sending turn to sync on for all workers
	for i := range workerNodes {
		workerNodes[i].synchroniseChannel <- latestTurn
	}
	//Suspending until workers synchronised
	for i := range workerNodes {
		<-workerNodes[i].synchroniseChannel
	}
	//Unsuspending workers
	for i := range workerNodes {
		workerNodes[i].synchroniseChannel <- 0
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
