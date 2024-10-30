package stubs

var ProcessTurnsHandler = "GolOperations.Process"

// var OperationsHandler = "GolOperations.Operations"
var JobHandler = "GolOperations.ListenToWork"
var ActionHandler = "Broker.Action"
var ActionReport = "Broker.ActionWithReport"
var ActionHandlerWorker = "GolOperations.Action"
var ActionReportWorker = "GolOperations.ActionWithReport"
var ConnectDistributor = "Broker.ConnectDistributor"
var ConnectWorker = "Broker.ConnectWorker"
var MakeChannel = "Broker.MakeChannel"
var Publish = "Broker.Publish"
var Report = "GolOperations.Report"
var UpdateWorld = "GolOperations.UpdateWorld"
var UpdateBroker = "Broker.UpdateBroker"
var UpdateWorker = "GolOperations.UpdateWorker"

//var UpdateWorker = "Broker.UpdateWorker"

const NoAction int = 0
const Save int = 1
const Quit int = 2
const Pause int = 3
const UnPause int = 4
const Kill int = 5
const Ticker int = 6

// REGISTER : DISTRIBUTOR
// SUBSCRIBE : WORKER

// (Broker -> Distributor)
// Applies for Save, Kill, Ticker

type Params struct {
	Threads     int
	ImageHeight int
	ImageWidth  int
	Turns       int
}

type Response struct {
	World     [][]uint8
	TurnsDone int
}

type Request struct {
	World       [][]uint8
	Threads     int
	Turns       int
	ImageWidth  int
	ImageHeight int
}

type WorkerRequest struct {
	WorkerId int
	StartY   int
	EndY     int
	StartX   int
	EndX     int
	World    [][]uint8
	Turns    int
	Params   Params
}

type PauseRequest struct {
	Pause bool
}

// (Distributor -> Broker)
// (Worker -> Broker)
type ChannelRequest struct {
	Threads int
}

// Connect to the broker from the first tiem and initialise world
type RegisterRequest struct {
	World       [][]uint8
	Threads     int
	ImageWidth  int
	ImageHeight int
}

type UpdateRequest struct {
	World    [][]uint8
	Turns    int
	WorkerId int
}

type ActionRequest struct {
	Action int
}

// ----------------- Keypresses --------------------

// (Broker -> Distributor)
// Applies for Save, Kill, Ticker

// (Worker -> Broker)
type SubscribeRequest struct {
	WorkerAddress string
}

// (Broker -> Worker)
type StateRequest struct {
	State int
}

// ----------------- Ticker -----------------------
// (Distributor -> Broker)
type TickerRequest struct {
}

// (Distributor -> Broker)
// (Broker -> Distributor)

// Response that doesn't require any additional data
type StatusReport struct {
	Status int
}

// 1. The distributor initialises the board, gets the input from the IO.
// 2. The distributor passes the value to the broker how many threads there would be.
// 3. The broker receives a request by 'ChannelRequest' and makes a channel, which communicates between workers and the broker.
// 4. The broker invokes a rpc call (GOLOperation) to the worker. 																=> rpc.GO
// 5. The worker accepts the rpc client, and iterates all the GOLOperation.
// 5-1. The distributor needs a report about calculating the number of the alive cell every 2 seconds.							=> goroutine for a (function)_loop
// (The counting alive function can be implemented either in the distributor, or in the worker.)
// 5-2. The distributor listens to a keypress, and if any action(keypress) has occured,											=> goroutine for a (function)_loop
//       we need to pass the action to the broker, and the broker sends the action to the worker.
