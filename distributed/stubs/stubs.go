package stubs

import "uk.ac.bris.cs/gameoflife/util"

var ProcessSlice = "WorkerOperations.ProcessSlice"
var BrokerRequest = "BrokerOperations.BrokerRequest"
/*var Subscribe = "BrokerOperations.Subscribe"

type SubscriptionRequest struct {
	IP string
}

type SubscriptionResponses struct {
	Message string
}*/

type Response struct {
	NextWorld [][]uint8
	AliveCells []util.Cell
}

type Request struct {
	CurrentWorld [][]uint8
	Turns int
}
