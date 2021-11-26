	package gol

import (
	"bufio"
	"fmt"
	"net/rpc"
	"os"
	"strconv"
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

var killChannel chan bool
var pauseChannel chan bool

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	killChannel = make(chan bool)
	pauseChannel = make(chan bool)
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
	defer func(client *rpc.Client) {
		err := client.Close()
		if err != nil {
			panic(err)
		}
	}(client)
	ticker := time.NewTicker(2*time.Second)
	go aliveCellsRetriever(client,c,ticker)
	go readKeys(c,client,p)
	req := stubs.Request{CurrentWorld: currentWorld, Turns: p.Turns}
	resp := new(stubs.Response)
	err = client.Call(stubs.BrokerRequest, req, resp)
	if err != nil {
		panic(err)
	}
	ticker.Stop()
	killChannel <- true

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
	close(c.events)
}

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
		panic(err)
	}
}

func readKeys(c distributorChannels, broker *rpc.Client, p Params) {
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
				killChannel <- true
				req := new(stubs.GenericMessage)
				resp := new(stubs.GenericMessage)
				err := broker.Call(stubs.KillBroker, req, resp)
				if err != nil {
					panic(err)
				}
				err = broker.Close()
				if err != nil {
					panic(err)
				}
			case 'q':
				fmt.Println("q")
				req := new(stubs.GenericMessage)
				resp := new(stubs.GenericMessage)
				err := broker.Call(stubs.DisconnectController, req, resp)
				if err != nil {
					panic(err)
				}
				err = broker.Close()
				if err != nil {
					panic(err)
				}
				break
			case 'p':
				fmt.Println("p")
				pauseChannel <- true
				req := new(stubs.GenericMessage)
				resp := new(stubs.PauseResponse)
				err := broker.Call(stubs.TogglePause, req, resp)
				if err != nil {
					panic(err)
				}
				if resp.Resuming {
					fmt.Println("Continuing")
				} else {
					fmt.Println(resp.Turn)
				}
			}
		default:
		}
	}
}

func getPGMFromServer(broker *rpc.Client, p Params, c distributorChannels) {
	req := new(stubs.GenericMessage)
	resp := new(stubs.PGMResponse)
	err := broker.Call(stubs.KeyPressPGM, req, resp)
	if err != nil {
		panic(err)
	}
	writeFile(p, c, resp.World, resp.Turns)
}

func aliveCellsRetriever(server *rpc.Client, c distributorChannels, ticker *time.Ticker) {
	for {
		select {
		case <-ticker.C:
			req := stubs.GenericMessage{}
			resp := new(stubs.AliveCellsResponse)

			err := server.Call(stubs.GetAliveCells, req, resp)
			if err != nil {
				panic(err)
			}
			c.events <- AliveCellsCount{resp.TurnsCompleted,resp.Cells}
			case <-pauseChannel:
				<-pauseChannel
			case <-killChannel:
				break
			default:
		}
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
