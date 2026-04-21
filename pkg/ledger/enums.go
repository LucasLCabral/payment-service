package ledger

type Direction string

const (
	DirectionCredit Direction = "CREDIT"
	DirectionDebit  Direction = "DEBIT"
)

type EntryStatus string

const (
	EntryStatusSettled  EntryStatus = "SETTLED"
	EntryStatusDeclined EntryStatus = "DECLINED"
)
