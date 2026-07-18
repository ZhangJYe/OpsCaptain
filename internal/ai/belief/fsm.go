package belief

import (
	"fmt"
	"sort"
)

type BeliefFSM struct {
	State        FSMState
	CurrentLevel int
	LevelSteps   map[int]int
	Thresholds   FSMThresholds
	History      []FSMTransition
	TotalSteps   int
}

func NewBeliefFSM(thresholds FSMThresholds) *BeliefFSM {
	return &BeliefFSM{
		State:        StateDrilling,
		CurrentLevel: 1,
		LevelSteps:   map[int]int{1: 0},
		Thresholds:   thresholds,
	}
}

func (f *BeliefFSM) GetState() FSMState {
	return f.State
}

func (f *BeliefFSM) GetCurrentLevel() int {
	return f.CurrentLevel
}

func (f *BeliefFSM) IsFinalState() bool {
	return f.State == StateDone
}

func (f *BeliefFSM) TickStep(k int) {
	f.TotalSteps += k
	f.LevelSteps[f.CurrentLevel] += k
}

func (f *BeliefFSM) Decide(g *BeliefGraph) *FSMDecision {
	if f.State == StateDone {
		return &FSMDecision{Action: "done", Reason: "already done"}
	}

	cands := f.topHypos(g, f.CurrentLevel)
	if len(cands) == 0 {
		return &FSMDecision{Action: "report", Reason: "no candidates"}
	}

	gap := f.gap(cands)
	sup := f.countSupport(g, cands[0].NodeID)
	steps := f.LevelSteps[f.CurrentLevel]

	if gap >= f.Thresholds.GapDelta && sup >= f.Thresholds.MinSupport {
		return &FSMDecision{Action: "report", Reason: fmt.Sprintf("gap=%.2f sup=%d", gap, sup)}
	}

	if steps >= f.Thresholds.MaxSteps {
		return &FSMDecision{Action: "report", Reason: fmt.Sprintf("max steps %d", steps)}
	}

	return &FSMDecision{Action: "continue", Reason: "conditions not met"}
}

func (f *BeliefFSM) TransitionTo(s FSMState, reason string) {
	f.transitionTo(s, f.CurrentLevel, f.CurrentLevel, reason)
}

func (f *BeliefFSM) transitionTo(s FSMState, fromLevel int, toLevel int, reason string) {
	f.History = append(f.History, FSMTransition{
		From:      f.State,
		To:        s,
		FromLevel: fromLevel,
		ToLevel:   toLevel,
		Reason:    reason,
		StepID:    fmt.Sprintf("step-%d", f.TotalSteps),
	})
	f.State = s
}

func (f *BeliefFSM) DrillDown(reason string) {
	fromLevel := f.CurrentLevel
	f.CurrentLevel++
	f.LevelSteps[f.CurrentLevel] = 0
	f.transitionTo(StateDrilling, fromLevel, f.CurrentLevel, reason)
}

func (f *BeliefFSM) BacktrackTo(level int, reason string) error {
	if f.State != StateDrilling {
		return fmt.Errorf("cannot backtrack from state %d", f.State)
	}
	if level < 1 || level >= f.CurrentLevel {
		return fmt.Errorf("backtrack level must be within [1,%d)", f.CurrentLevel)
	}
	fromLevel := f.CurrentLevel
	f.CurrentLevel = level
	if _, ok := f.LevelSteps[level]; !ok {
		f.LevelSteps[level] = 0
	}
	f.transitionTo(StateDrilling, fromLevel, f.CurrentLevel, reason)
	return nil
}

func (f *BeliefFSM) MarkDone(reason string) {
	f.TransitionTo(StateDone, reason)
}

func (f *BeliefFSM) topHypos(g *BeliefGraph, level int) []LevelNode {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var c []LevelNode
	for _, n := range g.Nodes {
		if n.Type == NodeHypothesis && n.Status == StatusActive && n.Level == level {
			c = append(c, LevelNode{NodeID: n.ID, Confidence: n.Score})
		}
	}
	sort.Slice(c, func(i, j int) bool {
		if c[i].Confidence == c[j].Confidence {
			return c[i].NodeID < c[j].NodeID
		}
		return c[i].Confidence > c[j].Confidence
	})
	return c
}

func (f *BeliefFSM) gap(c []LevelNode) float64 {
	if len(c) < 2 {
		return c[0].Confidence
	}
	return c[0].Confidence - c[1].Confidence
}

func (f *BeliefFSM) countSupport(g *BeliefGraph, id string) int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	n := 0
	for _, e := range g.Edges {
		if e.Dst == id && e.Type == EdgeSupport && e.Status == StatusActive {
			n++
		}
	}
	return n
}
