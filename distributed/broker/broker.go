package main

import (
	"fmt"
	"net"
	"net/rpc"
	"time"

	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

var workers []string
var workerClients []*rpc.Client
var controller *rpc.Client
var requestingPGM bool
var PGMChannel chan [][]uint8
var listener net.Listener
var paused bool
var pauseChannel chan bool
var turnChannel chan int

type BrokerOperations struct{}

func (b *BrokerOperations) SubscribeWorker(req stubs.SubscriptionRequest, resp *stubs.GenericMessage) (err error){
	workers = append(workers,req.IP)
	fmt.Println("Received subscription request from worker on "+req.IP)
	return
}

func (b *BrokerOperations) TogglePause(req stubs.GenericMessage, resp *stubs.PauseResponse) (err error){
	paused = !paused
	if(paused){
		resp.Resuming = false
		resp.Turn = <-turnChannel
	}else{
		<-pauseChannel
		resp.Resuming = true
	}
	return
}

func (b *BrokerOperations) Kill(req stubs.GenericMessage, resp *stubs.GenericMessage) (err error){
	for i := range workerClients{
		wReq := new(stubs.GenericMessage)
		wResp := new(stubs.GenericMessage)
		workerClients[i].Call(stubs.KillWorker,wReq,wResp)
	}
	listener.Close()
	return
}

func (b *BrokerOperations) KeyPressPGM(req stubs.GenericMessage, resp *stubs.PGMResponse) (err error){
	requestingPGM = true
	resp.World = <-PGMChannel
	return
}

func (b *BrokerOperations) SubscribeController(req stubs.SubscriptionRequest, resp *stubs.GenericMessage) (err error){
	controller,err = rpc.Dial("tcp", req.IP)
	if err != nil{
		fmt.Println("Failed to connect to controller on "+req.IP+" - "+err.Error())
	}else {
		fmt.Println("Connected to controller on "+req.IP)
	}
	return
}

func (b *BrokerOperations) BrokerRequest(req stubs.Request, resp *stubs.Response) (err error) {
	clientChannels := []chan [][]uint8{}
	workerClients = []*rpc.Client{}
	//Creating channels and connections to workers nodes
	for i := range workers {
		fmt.Println(workers[i])
		newClient, err := rpc.Dial("tcp", workers[i])
		if err != nil {
			fmt.Println("Broker dialing error on ", workers[i], " - ", err.Error())
		} else {
			fmt.Println("Broker connected to worker on ", workers[i])
			workerClients = append(workerClients, newClient)
			clientChannels = append(clientChannels, make(chan [][]byte))
		}
		//defer newClient.Close()
	}

	ticker := time.NewTicker(2 * time.Second)

	currentWorld := req.CurrentWorld
	turns := req.Turns
	columnsPerChannel := len(currentWorld) / len(workerClients)
	for turn := 0; turn < turns; turn++ {
		nextWorld := [][]byte{}
		//Splitting up world and distributing to channels
		remainders := len(currentWorld) % len(workerClients)
		offset := 0
		for sliceNum := 0; sliceNum < len(workerClients); sliceNum++ {
			go callWorker(clientChannels[sliceNum], workerClients[sliceNum])
			currentSlice := sliceWorld(sliceNum, columnsPerChannel, currentWorld, &remainders, &offset)
			clientChannels[sliceNum] <- currentSlice
		}
		//Reconstructing image from worker channels
		for i := range clientChannels {
			nextSlice := <-clientChannels[i]
			for j := range nextSlice {
				nextWorld = append(nextWorld, nextSlice[j])
			}
		}
		select {
		case <-ticker.C:
			cells := getAliveCellsCount(currentWorld)
			cellsReq := stubs.AliveCellsRequest{Cells: cells,TurnsCompleted: turn}
			cellsResp := new(stubs.GenericMessage)
			controller.Call(stubs.ReceiveAliveCells, cellsReq, cellsResp)
			fmt.Println("sent")
		default:
		}
		if requestingPGM{
			PGMChannel <- currentWorld
			requestingPGM = false
		}
		if paused{
			turnChannel<-turn
			pauseChannel<-true
		}
		if req.Turns > 0 {
			currentWorld = nextWorld
		}
	}
	ticker.Stop()
	resp.NextWorld = currentWorld

	//calculate the alive cells
	aliveCells := make([]util.Cell, 0, len(currentWorld)*len(currentWorld[0]))
	for i, _ := range currentWorld {
		for j, _ := range currentWorld[i] {
			if currentWorld[i][j] == 0xFF {
				newCell := util.Cell{X: j, Y: i}
				aliveCells = append(aliveCells, newCell)
			}
		}
	}

	resp.AliveCells = aliveCells

	return
}

func sliceWorld(sliceNum int, columnsPerChannel int, currentWorld [][]byte, remainderThreads *int, offset *int) [][]uint8 {
	currentSlice := [][]uint8{}
	//Adding extra column to back of slice to avoid lines of cells that aren't processed
	extraBackColumnIndex := boundNumber(sliceNum*columnsPerChannel-1+*offset, len(currentWorld))
	currentSlice = append(currentSlice, currentWorld[extraBackColumnIndex])
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
	extraFrontColumnIndex := boundNumber(sliceNum*columnsPerChannel+columnsPerChannel+*offset, len(currentWorld))
	currentSlice = append(currentSlice, currentWorld[extraFrontColumnIndex])
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

func callWorker(channel chan [][]uint8, workerClient *rpc.Client) {
	req := stubs.Request{CurrentWorld: <-channel}
	resp := new(stubs.Response)
	workerClient.Call(stubs.ProcessSlice, req, resp)
	channel <- resp.NextWorld
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

/*func (b *BrokerOperations) Subscribe(req stubs.SubscriptionRequest, resp *stubs.SubscriptionResponses) (err error){
	//check this for races
	workers = append(workers, req.IP)
	resp.Message = "Subscription received"
	return
}*/

func main() {
	requestingPGM = false
	paused = false
	PGMChannel = make(chan [][]uint8,1)
	pauseChannel = make(chan bool)
	turnChannel = make(chan int)
	rpc.Register(&BrokerOperations{})
	var err error
	listener, err = net.Listen("tcp", ":8040")
	if err != nil {
		fmt.Println("Broker listening error: ", err.Error())
	}
	rpc.Accept(listener)
}
