package transfer

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

type busyPartCatalog struct {
	*leaseCatalog
	claims int
	busy   int
	err    error
}

func (c *busyPartCatalog) ClaimPart(context.Context, uploads.ClaimPartInput) (*uploads.ClaimPartResult, error) {
	c.claims++
	if c.claims <= c.busy {
		return nil, uploads.ErrPartBusy
	}
	return &uploads.ClaimPartResult{Part: &sqlcgen.UploadPart{}, LeaseToken: c.leaseToken}, c.err
}

func TestClaimPartWaitsWithCappedBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		catalog := &busyPartCatalog{leaseCatalog: newLeaseCatalog(), busy: 6}
		pipeline := &Pipeline{catalog: catalog}
		started := time.Now()
		claim, err := pipeline.claimPart(context.Background(), uploads.ClaimPartInput{})
		if err != nil || claim.LeaseToken != catalog.leaseToken || claim.Existing {
			t.Fatalf("claimPart() = %+v, %v", claim, err)
		}
		// 250ms + 500ms + 1s + 2s + 2s + 2s.
		if elapsed := time.Since(started); elapsed != 7750*time.Millisecond || catalog.claims != 7 {
			t.Fatalf("elapsed = %v, claims = %d", elapsed, catalog.claims)
		}
	})
}

func TestClaimPartStopsOnCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		catalog := &busyPartCatalog{leaseCatalog: newLeaseCatalog(), busy: 10}
		pipeline := &Pipeline{catalog: catalog}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		started := time.Now()
		if _, err := pipeline.claimPart(ctx, uploads.ClaimPartInput{}); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("claimPart() = %v, want deadline exceeded", err)
		}
		if elapsed := time.Since(started); elapsed != 100*time.Millisecond || catalog.claims != 1 {
			t.Fatalf("elapsed = %v, claims = %d", elapsed, catalog.claims)
		}
	})
}

func TestClaimPartDoesNotRetryPermanentErrors(t *testing.T) {
	for _, want := range []error{uploads.ErrInvalidState, uploads.ErrPartConflict, uploads.ErrExpired} {
		t.Run(want.Error(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				catalog := &busyPartCatalog{leaseCatalog: newLeaseCatalog(), err: want}
				pipeline := &Pipeline{catalog: catalog}
				started := time.Now()
				if _, err := pipeline.claimPart(context.Background(), uploads.ClaimPartInput{}); !errors.Is(err, want) {
					t.Fatalf("claimPart() = %v, want %v", err, want)
				}
				if time.Since(started) != 0 || catalog.claims != 1 {
					t.Fatalf("retried permanent error: calls = %d", catalog.claims)
				}
			})
		})
	}
}
