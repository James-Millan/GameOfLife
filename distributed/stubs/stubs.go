package stubs

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
}

type Request struct {
	CurrentWorld [][]uint8
	Turns int
}
