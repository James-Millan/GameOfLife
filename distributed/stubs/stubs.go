package stubs

import "uk.ac.bris.cs/gameoflife/util"

var ProcessSlice = "WorkerOperations.ProcessSlice"
var BrokerRequest = "BrokerOperations.BrokerRequest"
var SubscribeWorker = "BrokerOperations.SubscribeWorker"
var SubscribeController = "BrokerOperations.SubscribeController"
var GetAliveCells = "BrokerOperations.GetAliveCells"
var KeyPressPGM = "BrokerOperations.KeyPressPGM"
var KillBroker = "BrokerOperations.Kill"
var KillWorker = "WorkerOperations.Kill"
var TogglePause = "BrokerOperations.TogglePause"
var DisconnectController = "BrokerOperations.DisconnectController"

type SubscriptionRequest struct {
	IP string
}

type PGMResponse struct{
	World [][]uint8
	Turns int
}

type AliveCellsResponse struct{
	Cells int
	TurnsCompleted int
}

type GenericMessage struct {
	Message string
}

type PauseResponse struct{
	Turn int
	Resuming bool
}

type Response struct {
	NextWorld [][]uint8
	AliveCells []util.Cell
}

type Request struct {
	CurrentWorld [][]uint8
	Turns int
}
