package ledger

import pkgledger "github.com/LucasLCabral/payment-service/pkg/ledger"

// opportunity to create a new service to handle limits
const maxAmountCents = 100_000_00

type evaluation struct {
	status pkgledger.EntryStatus
	reason string
}

func evaluate(evt *pkgledger.PaymentCreatedEvent, payerBalance int64) evaluation {
	if evt.AmountCents > maxAmountCents {
		return evaluation{
			status: pkgledger.EntryStatusDeclined,
			reason: "amount exceeds transaction limit",
		}
	}

	if payerBalance < evt.AmountCents {
		return evaluation{
			status: pkgledger.EntryStatusDeclined,
			reason: "insufficient funds",
		}
	}

	return evaluation{status: pkgledger.EntryStatusSettled}
}
