package stubs

import "uk.ac.bris.cs/gameoflife/util"

var ProcessSlice = "WorkerOperations.ProcessSlice"
var BrokerRequest = "BrokerOperations.BrokerRequest"
var SubscribeWorker = "BrokerOperations.SubscribeWorker"
var SubscribeController = "BrokerOperations.SubscribeController"
var ReceiveAliveCells = "ControllerOperations.ReceiveAliveCells"

type SubscriptionRequest struct {
	IP string
}

type AliveCellsRequest struct{
	Cells int
	TurnsCompleted int
}

type GenericResponse struct {
	Message string
}

type Response struct {
	NextWorld [][]uint8
	AliveCells []util.Cell
}

type Request struct {
	CurrentWorld [][]uint8
	Turns int
}
