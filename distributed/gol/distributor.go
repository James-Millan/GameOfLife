package gol

import (
	"bufio"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"strconv"

	"uk.ac.bris.cs/gameoflife/stubs"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

type ControllerOperations struct {}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	eventsChannel = c.events
	//Reading values from config
	file, rerr := os.Open("gol/config")
	if rerr != nil {
		fmt.Println("Error reading config file: "+rerr.Error())
	}
	reader := bufio.NewScanner(file)
	reader.Scan()
	myIp := reader.Text()
	reader.Scan()
	myPort := reader.Text()
	reader.Scan()
	brokerIp := reader.Text()
	file.Close()

	//Create a 2D slice to store the world
	currentWorld := make([][]byte, p.ImageWidth)
	for i := 0; i < p.ImageWidth; i++ {
		currentWorld[i] = make([]byte, p.ImageHeight)
	}

	//brokerIp := flag.String("broker", "127.0.0.1:8030", "IP address of broker")
	//flag.Parse()

	//TODO read in initial state of GOL using io.go
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
	//fine up to here (works on 0 turns)

	// TODO: Execute all turns of the Game of Life.
	turns := p.Turns

	client, err := rpc.Dial("tcp", string(brokerIp))
	if err != nil {
		fmt.Println("Distributor dialing error: ", err.Error())
	}
	defer client.Close()

	rpc.Register(&ControllerOperations{})
	aliveListener,aerr := net.Listen("tcp",":"+myPort)
	if aerr != nil{
		fmt.Println("Distributor listening error: "+aerr.Error())
	}
	go aliveCellsListen(aliveListener)
	subRequest := stubs.SubscriptionRequest{IP: myIp+":"+myPort}
	subResp := new(stubs.GenericResponse)
	client.Call(stubs.SubscribeController,subRequest,subResp)
	req := stubs.Request{CurrentWorld: currentWorld, Turns: p.Turns}
	resp := new(stubs.Response)
	client.Call(stubs.BrokerRequest, req, resp)

	aliveListener.Close()
	//currentWorld = resp.NextWorld

	// TODO: Report the final state using FinalTurnCompleteEvent.
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

func aliveCellsListen(listener net.Listener){
	rpc.Accept(listener)
}

var eventsChannel chan <- Event

func (b *ControllerOperations) ReceiveAliveCells(req stubs.AliveCellsRequest, resp *stubs.GenericResponse) (err error){

	eventsChannel <- AliveCellsCount{CellsCount: req.Cells,CompletedTurns: req.TurnsCompleted}
	eventsChannel <- TurnComplete{CompletedTurns: req.TurnsCompleted}
	return
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
