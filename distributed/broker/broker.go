package main

import (
	"fmt"
	"net"
	"net/rpc"
	"os"
	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

var workers []string

type BrokerOperations struct{}

func (b *BrokerOperations) BrokerRequest(req stubs.Request, resp *stubs.Response) (err error) {
	fmt.Println("broker received")
	clientChannels := []chan [][]uint8{}
	for range workers{
		clientChannels = append(clientChannels,make(chan [][]byte))
	}

	currentWorld := req.CurrentWorld
	turns := req.Turns
	columnsPerChannel := len(currentWorld) / len(workers)
	for turn := 0; turn < turns; turn++ {
		nextWorld := [][]byte{}
		//Splitting up world and distributing to channels
		remainders := len(currentWorld) % len(workers)
		offset := 0
		for sliceNum := 0; sliceNum < len(workers); sliceNum++{
			go callWorker(clientChannels[sliceNum],workers[sliceNum])
			currentSlice := sliceWorld(sliceNum,columnsPerChannel,currentWorld,&remainders,&offset)
			clientChannels[sliceNum] <- currentSlice
		}
		//Reconstructing image from worker channels
		for i := range clientChannels{
			nextSlice := <- clientChannels[i]
			for j := range nextSlice{
				nextWorld = append(nextWorld, nextSlice[j])
			}
		}
		if req.Turns > 0{
			currentWorld = nextWorld
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

func sliceWorld(sliceNum int,columnsPerChannel int,currentWorld [][]byte,remainderThreads *int,offset *int) [][]uint8{
	currentSlice := [][]uint8{}
	//Adding extra column to back of slice to avoid lines of cells that aren't processed
	extraBackColumnIndex := boundNumber(sliceNum * columnsPerChannel - 1 + *offset,len(currentWorld))
	currentSlice = append(currentSlice,currentWorld[extraBackColumnIndex])
	for i := 0;i < columnsPerChannel;i++{
		currentSlice = append(currentSlice,currentWorld[boundNumber(sliceNum * columnsPerChannel + i + *offset,len(currentWorld))])
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

func callWorker(channel chan [][]uint8,workerIp string){
	client, err := rpc.Dial("tcp", workerIp)
	if err != nil {
		fmt.Println("Broker dialing error on ",workerIp," - ", err.Error())
	}
	defer client.Close()
	req := stubs.Request{CurrentWorld: <- channel}
	resp := new(stubs.Response)
	client.Call(stubs.ProcessSlice, req, resp)
	channel <- resp.NextWorld
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

/*func (b *BrokerOperations) Subscribe(req stubs.SubscriptionRequest, resp *stubs.SubscriptionResponses) (err error){
	//check this for races
	workers = append(workers, req.IP)
	resp.Message = "Subscription received"
	return
}*/

func main() {
	workers = os.Args[1:]
	if len(workers) > 0 {
		rpc.Register(&BrokerOperations{})
		listener, err := net.Listen("tcp", ":8030")
		if err != nil {
			fmt.Println("Broker listening error: ", err.Error())
		}
		defer listener.Close()
		rpc.Accept(listener)
	}else{
		fmt.Println("Please give IP for workers as arguments")
	}
}
