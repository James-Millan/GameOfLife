package main

import (
	"fmt"
	"net"
	"net/rpc"
	"os"

	"uk.ac.bris.cs/gameoflife/stubs"
)

var workers []string

type BrokerOperations struct{}

func (b *BrokerOperations) BrokerRequest(req stubs.Request, resp *stubs.Response) (err error) {
	fmt.Println("broker recieved")
	return
}

func main() {
	workers = os.Args[:1]
	rpc.Register(&BrokerOperations{})
	listener, err := net.Listen("tcp", ":8030")
	if err != nil {
		fmt.Println("Broker listening error: ", err.Error())
	}
	defer listener.Close()
	rpc.Accept(listener)
}
