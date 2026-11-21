package source

import "context"

// RawLedgerFunc receives one ledger's undecoded XDR, in sequence order.
//
// raw is only valid until the function returns. The underlying stream reuses
// the slice on its next step, so anything kept beyond the call must be
// copied. Returning an error stops the stream and that error reaches the
// caller of [RawLedgerSource.StreamRaw] unwrapped.
type RawLedgerFunc func(sequence uint32, raw []byte) error

// RawLedgerSource is implemented by a [LedgerSource] that can hand out each
// ledger's bytes without decoding them.
//
// It is a separate interface rather than a method on LedgerSource, for the
// same reason as [WindowedSource]: LedgerSource is part of the stable surface
// and adding a method there would break every external implementation.
// Callers type-assert for this one, so a source that cannot answer simply
// does not implement it.
//
// It exists because decoding is wasted work for a consumer that only wants to
// store or forward the bytes. `nebu fetch` writes raw ledger XDR, and going
// through [LedgerSource.Stream] would decode every ledger and re-encode it —
// paying for the round trip on nebu's bulk-transfer path, and making the
// output depend on the encoder rather than on what the source served.
//
// The callback is deliberate. A channel of []byte would hand the receiver
// memory that the stream overwrites on its next step, and nothing about the
// resulting corruption would be loud.
type RawLedgerSource interface {
	// StreamRaw streams ledgers [start, end] to fn and reports how many it
	// delivered. end == 0 means unbounded.
	//
	// The count is part of the contract: a bounded range that returns fewer
	// ledgers than it asked for has not failed in any way the error can show,
	// so the caller is the only thing that can notice.
	StreamRaw(ctx context.Context, start, end uint32, fn RawLedgerFunc) (uint32, error)
}
