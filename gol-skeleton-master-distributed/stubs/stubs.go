package stubs

const (
	Process            = "Broker.Process"
	GetAliveCells      = "Broker.GetAliveCells"
	StopProcessing     = "Broker.StopProcessing"
	GetWorld           = "Broker.GetWorld"
	Pause              = "Broker.Pause"
	Resume             = "Broker.Resume"
	Shutdown           = "Broker.Shutdown"
	CalculateNextState = "GolWorker.CalculateNextState"
)

type EngineRequest struct {
	World       [][]uint8
	ImageWidth  int
	ImageHeight int
	Turns       int
}

type EngineResponse struct {
	World          [][]uint8
	CompletedTurns int
}

type AliveCellsCountRequest struct{}

type AliveCellsCountResponse struct {
	CompletedTurns int
	CellsCount     int
}

type StopRequest struct{}

type StopResponse struct{}

type GetWorldRequest struct{}

type GetWorldResponse struct {
	World          [][]uint8
	CompletedTurns int
	Processing     bool
}

type PauseRequest struct{}

type PauseResponse struct {
	Turn int
}

type ResumeRequest struct{}

type ResumeResponse struct{}

type ShutdownRequest struct{}

type ShutdownResponse struct{}

type WorkerRequest struct {
	StartY      int
	EndY        int
	WorldSlice  [][]uint8
	ImageWidth  int
	ImageHeight int
}

type WorkerResponse struct {
	WorldSlice [][]uint8
}
