package main

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"sync"

	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

var listener net.Listener
var workers []string
var workerClients []*rpc.Client
var clientChannels []chan [][]uint8
var safetyChannels []chan bool

var requestingPGM bool
var PGMChannel chan [][]uint8

var paused bool
var pauseChannel chan bool
var turnChannel chan int
var cellsChannel chan int

var pgmMutex sync.Mutex
var killMutex sync.Mutex

var requestingShutdown bool
var shutdownChannel chan bool
var stopCallChannel chan bool

type BrokerOperations struct{}

func (b *BrokerOperations) SubscribeWorker(req stubs.SubscriptionRequest, resp *stubs.GenericMessage) (err error) {
	workers = append(workers, req.IP)
	fmt.Println("Received subscription request from worker on " + req.IP)
	return
}

func (b *BrokerOperations) TogglePause(req stubs.GenericMessage, resp *stubs.PauseResponse) (err error) {
	paused = !paused
	if paused {
		pauseChannel <- true
		resp.Resuming = false
		resp.Turn = <-turnChannel
	} else {
		<-pauseChannel
		resp.Resuming = true
	}
	return
}

func (b *BrokerOperations) Kill(req stubs.GenericMessage, resp *stubs.GenericMessage) (err error) {
	killMutex.Lock()
	requestingShutdown = true
	killMutex.Unlock()
	<-shutdownChannel
	for i := range workerClients {
		wReq := new(stubs.GenericMessage)
		wResp := new(stubs.GenericMessage)
		workerClients[i].Call(stubs.KillWorker, wReq, wResp)
		/*if err != nil {
			fmt.Println(err)
		}*/
	}
	err = listener.Close()
	if err != nil {
		fmt.Println(err)
	}
	return
}

func (b *BrokerOperations) KeyPressPGM(req stubs.GenericMessage, resp *stubs.PGMResponse) (err error) {
	pgmMutex.Lock()
	requestingPGM = true
	pgmMutex.Unlock()
	resp.World = <-PGMChannel
	resp.Turns = <-turnChannel
	return
}

/*func (b *BrokerOperations) SubscribeController(req stubs.SubscriptionRequest, resp *stubs.GenericMessage) (err error){
	controller,err = rpc.Dial("tcp", req.IP)
	if err != nil{
		fmt.Println("Failed to connect to controller on "+req.IP+" - "+err.Error())
	}else {
		fmt.Println("Connected to controller on "+req.IP)
	}
	return
}*/

func (b *BrokerOperations) GetAliveCells(req stubs.GenericMessage, resp *stubs.AliveCellsResponse) (err error) {
	cellsChannel <- 0
	resp.Cells = <-cellsChannel
	resp.TurnsCompleted = <-turnChannel
	return
}

func (b *BrokerOperations) DisconnectController(req stubs.GenericMessage, resp *stubs.GenericMessage) (err error) {
	stopCallChannel <- true
	return
}

func attemptConnectWorkers(){
	if len(workerClients) > 0 {
		for i := range workerClients{
			workerClients[i].Close()
		}
	}
	workerClients = []*rpc.Client{}
	clientChannels = []chan [][]uint8{}
	safetyChannels = []chan bool{}
	for i := range workers {
		fmt.Println("Attempting to connect to worker on ",workers[i])
		newClient, derr := rpc.Dial("tcp", workers[i])
		if derr != nil {
			fmt.Println("Broker dialing error on ", workers[i], " - ", derr.Error())
		} else {
			fmt.Println("Broker connected to worker on ", workers[i])
			workerClients = append(workerClients, newClient)
			clientChannels = append(clientChannels, make(chan [][]byte))
			safetyChannels = append(safetyChannels, make(chan bool))
		}
	}
}

func checkFaults(currentWorld [][]uint8){
	fault := false
	for i := range safetyChannels{
		if <-safetyChannels[i] == false{
			fault = true
		}
	}
	if fault {
		fmt.Println("Fault detected, retrying turn")
		//Cleaning successful channels
		for i:= range clientChannels {
			<-clientChannels[i]
		}
		attemptConnectWorkers()
		distributeWorkers(currentWorld)
		checkFaults(currentWorld)
	}
}

func (b *BrokerOperations) BrokerRequest(req stubs.Request, resp *stubs.Response) (err error) {
	attemptConnectWorkers()
	//Creating channels and connections to workers nodes


	currentWorld := req.CurrentWorld
	turns := req.Turns
	breakLoop := false
	for turn := 0; turn < turns; turn++ {
		var nextWorld [][]byte
		//Splitting up world and distributing to channels
		distributeWorkers(currentWorld)
		//Checking for faults
		checkFaults(currentWorld)
		//Reconstructing image from worker channels
		for i := range clientChannels {
			nextSlice := <-clientChannels[i]
			for j := range nextSlice {
				nextWorld = append(nextWorld, nextSlice[j])
			}
		}
		/*select {
		case <-ticker.C:
			cells := getAliveCellsCount(currentWorld)
			cellsReq := stubs.AliveCellsRequest{Cells: cells,TurnsCompleted: turn}
			cellsResp := new(stubs.GenericMessage)
			controller.Call(stubs.ReceiveAliveCells, cellsReq, cellsResp)
			fmt.Println("sent")
		default:
		}*/
		select {
		case <-cellsChannel:
			cellsChannel <- getAliveCellsCount(currentWorld)
			turnChannel <- turn
		default:
		}
		pgmMutex.Lock()
		if requestingPGM {
			PGMChannel <- currentWorld
			turnChannel <- turn
			requestingPGM = false
		}
		pgmMutex.Unlock()
		killMutex.Lock()
		if requestingShutdown {
			shutdownChannel <- true
			requestingShutdown = false
			break
		}
		killMutex.Unlock()
		select {
		case <-pauseChannel:
			turnChannel <- turn
			pauseChannel <- true
		default:
		}
		if req.Turns > 0 {
			currentWorld = nextWorld
		}
		select {
		case <-stopCallChannel:
			breakLoop = true
		default:
		}
		if breakLoop {
			break
		}
	}
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

func distributeWorkers(currentWorld [][]byte){
	columnsPerChannel := len(currentWorld) / len(workerClients)
	remainders := len(currentWorld) % len(workerClients)
	offset := 0
	for sliceNum := 0; sliceNum < len(workerClients); sliceNum++ {
		go callWorker(clientChannels[sliceNum], workerClients[sliceNum],safetyChannels[sliceNum])
		currentSlice := sliceWorld(sliceNum, columnsPerChannel, currentWorld, &remainders, &offset)
		clientChannels[sliceNum] <- currentSlice
	}
}

func sliceWorld(sliceNum int, columnsPerChannel int, currentWorld [][]byte, remainderThreads *int, offset *int) [][]uint8 {
	var currentSlice [][]uint8
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

func callWorker(channel chan [][]uint8, workerClient *rpc.Client,safetyChannel chan bool) {
	req := stubs.Request{CurrentWorld: <-channel}
	resp := new(stubs.Response)
	err := workerClient.Call(stubs.ProcessSlice, req, resp)
	if err != nil {
		safetyChannel <- false
		channel <- nil
	}else {
		safetyChannel <- true
		channel <- resp.NextWorld
	}
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
	pgmMutex = sync.Mutex{}
	killMutex = sync.Mutex{}
	paused = false
	requestingPGM = false
	requestingShutdown = false
	PGMChannel = make(chan [][]uint8, 1)
	pauseChannel = make(chan bool)
	shutdownChannel = make(chan bool)
	turnChannel = make(chan int)
	cellsChannel = make(chan int)
	stopCallChannel = make(chan bool)
	err := rpc.Register(&BrokerOperations{})
	if err != nil {
		fmt.Println(err)
	}
	port := flag.String("port","8040","Port broker will listen on")
	flag.Parse()
	listener, err = net.Listen("tcp", ":"+*port)
	if err != nil {
		fmt.Println("Broker listening error: ", err.Error())
	}
	rpc.Accept(listener)
}
