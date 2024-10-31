package stubs

const (
	Process        = "GolEngine.Process"
	GetAliveCells  = "GolEngine.GetAliveCells"
	StopProcessing = "GolEngine.StopProcessing"
	GetWorld       = "GolEngine.GetWorld"
	Pause          = "GolEngine.Pause"
	Resume         = "GolEngine.Resume"
	Shutdown       = "GolEngine.Shutdown"
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
