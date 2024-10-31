package stubs

var ProcessTurnsHandler = "GolOperations.Process"
var OperationsHandler = "GolOperations.Operations"

const Save int = 0
const Quit int = 1
const Pause int = 2
const UnPause int = 3

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

type JobRequest struct {
	Job int
}
