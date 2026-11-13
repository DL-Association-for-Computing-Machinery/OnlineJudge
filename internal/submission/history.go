package submission

import "time"

// PersonalTrainingScope is the fixed context for ordinary personal training.
const PersonalTrainingScope = "personal-training"

// HistoryPosition locates the next page after the last visible timestamp and ID.
type HistoryPosition struct {
	CreatedAt time.Time
	ID        string
}

// HistoryPage contains summaries only. Next is nil at the end of the list.
type HistoryPage struct {
	Items []Receipt
	Next  *HistoryPosition
}
