package gol

import (
	"bufio"
	"fmt"
	"net/rpc"
	"os"
	"strconv"
	"sync"
	"time"

	"uk.ac.bris.cs/gameoflife/stubs"
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

var eventsRoutineKilled bool
var killLock sync.Mutex
var killChannel chan bool
var channelClosedLock sync.Mutex
var eventsChannelClosed bool

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	eventsRoutineKilled = false
	eventsChannelClosed = false
	killLock = sync.Mutex{}
	channelClosedLock = sync.Mutex{}
	killChannel = make(chan bool)
	var brokerIp string
	readConfigFile(&brokerIp)

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
	for j, _ := range currentWorld {
		for k, _ := range currentWorld[j] {
			currentWorld[j][k] = <-c.ioInput
		}
	}
	//Execute all turns of the Game of Life.

	turns := p.Turns
	client, err := rpc.Dial("tcp", string(brokerIp))
	if err != nil {
		fmt.Println("Distributor dialing error: ", err.Error())
	}
	defer client.Close()
	ticker := time.NewTicker(2 * time.Second)
	go eventsRoutine(client, p, c, ticker)
	req := stubs.Request{CurrentWorld: currentWorld, Turns: p.Turns}
	resp := new(stubs.Response)
	err = client.Call(stubs.BrokerRequest, req, resp)
	ticker.Stop()
	killLock.Lock()
	if !eventsRoutineKilled {
		killChannel <- true
		<-killChannel
	}
	killLock.Unlock()

	//Report the final state using FinalTurnCompleteEvent.
	c.events <- FinalTurnComplete{
		CompletedTurns: turns,
		Alive:          resp.AliveCells}
	writeFile(p, c, resp.NextWorld, turns)
	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turns, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	channelClosedLock.Lock()
	eventsChannelClosed = true
	channelClosedLock.Unlock()
	close(c.events)
}

//Reads the broker's ip from gol/config and set brokerIp to it
func readConfigFile(brokerIp *string) {
	file, rerr := os.Open("gol/config")
	if rerr != nil {
		fmt.Println("Error reading config file: " + rerr.Error())
	}
	reader := bufio.NewScanner(file)
	reader.Scan()
	*brokerIp = reader.Text()
	err := file.Close()
	if err != nil {
		fmt.Println(err)
	}
}

//goroutine for event handling
func eventsRoutine(broker *rpc.Client, p Params, c distributorChannels, ticker *time.Ticker) {
	breakloop := false
	paused := false
	for {
		select {
		case command := <-c.keyPresses:
			switch command {
			case 's':
				fmt.Println("s")
				getPGMFromServer(broker, p, c)
			case 'k':
				fmt.Println("k")
				getPGMFromServer(broker, p, c)
				killLock.Lock()
				eventsRoutineKilled = true
				killLock.Unlock()
				req := new(stubs.GenericMessage)
				resp := new(stubs.GenericMessage)
				broker.Call(stubs.KillBroker, req, resp)
				broker.Close()
				breakloop = true

			case 'q':
				fmt.Println("q")
				killLock.Lock()
				eventsRoutineKilled = true
				killLock.Unlock()
				req := new(stubs.GenericMessage)
				resp := new(stubs.GenericMessage)
				err := broker.Call(stubs.DisconnectController, req, resp)
				if err != nil {
					fmt.Println(err)
				}
				err = broker.Close()
				if err != nil {
					fmt.Println(err)
				}
				breakloop = true
			case 'p':
				fmt.Println("p")
				req := new(stubs.GenericMessage)
				resp := new(stubs.PauseResponse)
				err := broker.Call(stubs.TogglePause, req, resp)
				if err != nil {
					fmt.Println(err)
				}
				if resp.Resuming {
					fmt.Println("Continuing")
					paused = false
				} else {
					fmt.Println(resp.Turn)
					paused = true
				}
			}
		case <-ticker.C:
			if !paused {
				req := stubs.GenericMessage{}
				resp := new(stubs.AliveCellsResponse)
				err := broker.Call(stubs.GetAliveCells, req, resp)
				if err != nil {
					fmt.Println(err)
				}
				channelClosedLock.Lock()
				if !eventsChannelClosed {
					c.events <- AliveCellsCount{resp.TurnsCompleted, resp.Cells}
				}
				channelClosedLock.Unlock()
			}
		case <-killChannel:
			breakloop = true
		default:
		}
		if breakloop {
			if !eventsRoutineKilled {
				killChannel <- true
			}
			break
		}
	}
}

func getPGMFromServer(broker *rpc.Client, p Params, c distributorChannels) {
	req := new(stubs.GenericMessage)
	resp := new(stubs.PGMResponse)
	err := broker.Call(stubs.KeyPressPGM, req, resp)
	if err != nil {
		fmt.Println(err)
	}
	writeFile(p, c, resp.World, resp.Turns)
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
