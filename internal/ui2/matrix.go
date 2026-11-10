package ui2

type RiskMatrixCell struct {
	Count int
}

type RiskThresholds struct {
	LowMax  int
	HighMin int
}

func DefaultThresholds() RiskThresholds { return RiskThresholds{LowMax: 5, HighMin: 12} }

func BuildRiskMatrixCells(counts map[[2]int]int) []RiskMatrixCell {
	cells := make([]RiskMatrixCell, 25)
	for i, likelihood := range []int{5, 4, 3, 2, 1} {
		for j, consequence := range []int{1, 2, 3, 4, 5} {
			cells[i*5+j].Count = counts[[2]int{likelihood, consequence}]
		}
	}
	return cells
}
