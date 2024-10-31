package stubs

var ProcessTurnsHandler = "GolOperations.Process"
var OperationsHandler = "GolOperations.Operations"
var KillingHandler = "GolOperations.ListenToQuit"
var PauseHandler = "GolOperations.ListenToPause"
var BrokerAndWorker = "Broker.ConnectWorker"
var BrokerAndDistributor = "Broker.ConnectDistributor"
var BrokerChannel = "Broker.MakeChannel"

type Response struct {
	World     [][]uint8
	TurnsDone int
}

type Request struct {
	World       [][]uint8
	Turns       int
	ImageWidth  int
	ImageHeight int
}

type KillRequest struct {
	Kill int
}

type PauseRequest struct {
	Pause bool
}

type ChannelRequest struct {
	Threads     int
	ImageWidth  int
	ImageHeight int
}

type StatusReport struct {
	Status string
}
