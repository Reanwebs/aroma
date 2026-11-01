package app

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"
)

// StartDailyAuctionPriceScheduler refreshes and stores the market snapshot at
// 10:00 Asia/Kolkata. If the server was offline at 10:00, it catches up once
// when the process starts, then resumes the daily schedule.
func StartDailyAuctionPriceScheduler(ctx context.Context, db *gorm.DB, onError func(error)) {
	if onError == nil {
		onError = func(err error) { log.Printf("daily Spices Board price refresh failed: %v", err) }
	}
	go func() {
		loc, err := time.LoadLocation("Asia/Kolkata")
		if err != nil {
			onError(err)
			return
		}
		refreshIfMissing := func() {
			now := time.Now().In(loc)
			tenAM := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, loc)
			if now.Before(tenAM) {
				return
			}
			today := now.Format("2006-01-02")
			var exists bool
			if err := db.WithContext(ctx).Raw("SELECT EXISTS(SELECT 1 FROM market_price_snapshots WHERE snapshot_date=?)", today).Scan(&exists).Error; err != nil {
				onError(err)
				return
			}
			if exists {
				return
			}
			fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			feed, err := fetchSmallCardamomPrices(fetchCtx)
			if err != nil {
				onError(err)
				return
			}
			if err := saveAuctionPriceSnapshot(fetchCtx, db, feed); err != nil {
				onError(err)
			}
		}
		refreshIfMissing()
		for {
			now := time.Now().In(loc)
			next := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, loc)
			if !now.Before(next) {
				next = next.AddDate(0, 0, 1)
			}
			timer := time.NewTimer(time.Until(next))
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			case <-timer.C:
				refreshIfMissing()
			}
		}
	}()
}
