package ui

type CounterTo struct {
	Current, Total int64
}

func StartCountTo(total int64) CounterTo {
	return CounterTo{Total: total}
}
