package checkpointentries

import (
	"strings"
	"testing"
)

func TestValidateCheckpoint(t *testing.T) {
	tests := []struct {
		name     string
		sequence uint32
		wantErr  bool
		// wantNearest, when set, must appear in the error: a message that
		// says "not a checkpoint" without saying which ledger to ask for
		// leaves the reader to work out the modular arithmetic.
		wantNearest []string
	}{
		{
			name:     "a checkpoint ledger",
			sequence: 5727039, // 5727040 mod 64 == 0
		},
		{
			name:     "the first checkpoint",
			sequence: 63,
		},
		{
			name:        "one past a checkpoint",
			sequence:    5727040,
			wantErr:     true,
			wantNearest: []string{"5727039", "5727103"},
		},
		{
			name:        "one before a checkpoint",
			sequence:    5727038,
			wantErr:     true,
			wantNearest: []string{"5726975", "5727039"},
		},
		{
			// The genesis ledger is not a checkpoint, and the arithmetic must
			// not underflow on the way to saying so.
			name:        "ledger 1",
			sequence:    1,
			wantErr:     true,
			wantNearest: []string{"63"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCheckpoint(tt.sequence)
			if tt.wantErr != (err != nil) {
				t.Fatalf("ValidateCheckpoint(%d) error = %v, want error: %v", tt.sequence, err, tt.wantErr)
			}
			if err == nil {
				return
			}
			for _, nearest := range tt.wantNearest {
				if !strings.Contains(err.Error(), nearest) {
					t.Errorf("error does not name %s: %v", nearest, err)
				}
			}
		})
	}
}

func TestNewReaderRejectsUnknownBucketList(t *testing.T) {
	tests := []struct {
		name       string
		bucketList string
		wantErr    bool
	}{
		{name: "live", bucketList: BucketListLive},
		{name: "hot", bucketList: BucketListHot},
		{name: "empty", bucketList: "", wantErr: true},
		{
			// Rejecting an unknown name beats defaulting to live: a typo
			// would otherwise return current state where the caller asked for
			// archived entries, and the counts would look plausible.
			name:       "a name that is neither",
			bucketList: "warm",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewReader(
				[]string{"https://history.stellar.org/prd/core-testnet/core_testnet_001/"},
				"Test SDF Network ; September 2015",
				"testnet",
				"v0.0.0-test",
				63,
				tt.bucketList,
			)
			if tt.wantErr != (err != nil) {
				t.Fatalf("NewReader(bucketList=%q) error = %v, want error: %v", tt.bucketList, err, tt.wantErr)
			}
		})
	}
}
