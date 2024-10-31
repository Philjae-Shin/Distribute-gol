package stubs

var ProcessTurnsHandler = "GolOperations.Process"
var OperationsHandler = "GolOperations.Operations"
var KillingHandler = "GolOperations.ListenToQuit"

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
