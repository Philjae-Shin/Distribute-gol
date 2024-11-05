// stubs.go

package stubs

import "uk.ac.bris.cs/gameoflife/util"

var Process = "Broker.Process"
var GetAliveCells = "Broker.GetAliveCells"
var StopProcessing = "Broker.StopProcessing"
var GetWorld = "Broker.GetWorld"
var Pause = "Broker.Pause"
var Resume = "Broker.Resume"
var Shutdown = "Broker.Shutdown"
var CalculateNextState = "GolWorker.CalculateNextState"
var GetTurnUpdates = "Broker.GetTurnUpdates"

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
	WorldSlice   [][]uint8
	CellsFlipped []util.Cell
}

type TurnUpdatesRequest struct{}

type TurnUpdatesResponse struct {
	CellsFlipped   []util.Cell
	CompletedTurns int
}
