package stubs

var ProcessSlice = "WorkerOperations.ProcessSlice"
var BrokerRequest = "BrokerOperations.BrokerRequest"

type Response struct {
	NextWorld [][]uint8
}

type Request struct {
	CurrentWorld [][]uint8
	Turns        int
}
