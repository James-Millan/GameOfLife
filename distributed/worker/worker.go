package main

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"uk.ac.bris.cs/gameoflife/stubs"
)

func main() {
	myIp := flag.String("ip","localhost","Worker's ip")
	port := flag.String("port","8050","Port worker will listen on")
	brokerIp := flag.String("brokerIp","localhost:8040","Address to connect to broker")
	flag.Parse()
	subscriber, serr := rpc.Dial("tcp",*brokerIp)
	if serr != nil {
		fmt.Println("Worker on "+*myIp+":"+*port+" failed to connect to broker on "+*brokerIp+" - "+serr.Error())
	}
	subRequest := stubs.SubscriptionRequest{IP: *myIp+":"+*port}
	subResp := new(stubs.GenericResponse)
	subscriber.Call(stubs.SubscribeWorker,subRequest,subResp)
	subscriber.Close()
	rpc.Register(&WorkerOperations{})
	listener, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		fmt.Println("Worker listening error: ", err.Error())
	}
	defer listener.Close()
	rpc.Accept(listener)
}

type WorkerOperations struct{}

func (w *WorkerOperations) ProcessSlice(req stubs.Request, resp *stubs.Response) (err error) {
	fmt.Println("Recieved")

	currentSlice := req.CurrentWorld
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
	resp.NextWorld = nextSlice
	return
}

//count number of active cells surrounding a current cell
func getNumSurroundingCells(x int, y int, world [][]byte) int {
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
