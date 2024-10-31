package stubs

const (
	Process        = "GolEngine.Process"
	GetAliveCells  = "GolEngine.GetAliveCells"
	StopProcessing = "GolEngine.StopProcessing"
	GetWorld       = "GolEngine.GetWorld"
)

type EngineRequest struct {
	World       [][]uint8
	ImageWidth  int
	ImageHeight int
	Turns       int
}

type EngineResponse struct {
	World [][]uint8
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
}
