package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/html"
	"gorm.io/gorm"
)

const smallCardamomArchiveURL = "https://www.indianspices.com/marketing/price/domestic/daily-price-small.html"

type AuctionPrice struct {
	Date        string `json:"date"`
	Auctioneer  string `json:"auctioneer"`
	AverageRate string `json:"average_rate"`
}

type AuctionPriceFeed struct {
	Items     []AuctionPrice `json:"items"`
	Source    string         `json:"source"`
	FetchedAt time.Time      `json:"fetched_at"`
}

func fetchSmallCardamomPrices(ctx context.Context) (AuctionPriceFeed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, smallCardamomArchiveURL, nil)
	if err != nil {
		return AuctionPriceFeed{}, err
	}
	req.Header.Set("User-Agent", "AromaDryer/1.0 (Spices Board daily price reader)")
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return AuctionPriceFeed{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return AuctionPriceFeed{}, fmt.Errorf("Spices Board returned HTTP %d", resp.StatusCode)
	}
	doc, err := html.Parse(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return AuctionPriceFeed{}, err
	}
	items := parseSmallCardamomPrices(doc)
	if len(items) == 0 {
		return AuctionPriceFeed{}, errors.New("could not find current small-cardamom auction prices")
	}
	return AuctionPriceFeed{Items: items, Source: smallCardamomArchiveURL, FetchedAt: time.Now().UTC()}, nil
}

type savedAuctionSnapshot struct {
	ID           uuid.UUID `gorm:"column:id"`
	SnapshotDate time.Time `gorm:"column:snapshot_date"`
	FetchedAt    time.Time `gorm:"column:fetched_at"`
	Source       string    `gorm:"column:source"`
}

type savedAuctionPrice struct {
	Position    int       `gorm:"column:position"`
	AuctionDate time.Time `gorm:"column:auction_date"`
	Auctioneer  string    `gorm:"column:auctioneer"`
	AverageRate string    `gorm:"column:average_rate"`
}

func saveAuctionPriceSnapshot(ctx context.Context, db *gorm.DB, feed AuctionPriceFeed) error {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return err
	}
	snapshotDate := feed.FetchedAt.In(loc).Format("2006-01-02")
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var id uuid.UUID
		err := tx.Raw(`INSERT INTO market_price_snapshots(snapshot_date, fetched_at, source)
			VALUES (?, ?, ?) ON CONFLICT(snapshot_date) DO UPDATE SET fetched_at=EXCLUDED.fetched_at, source=EXCLUDED.source
			RETURNING id`, snapshotDate, feed.FetchedAt, feed.Source).Row().Scan(&id)
		if err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM market_price_snapshot_rows WHERE snapshot_id=?", id).Error; err != nil {
			return err
		}
		for i, item := range feed.Items {
			auctionDate, err := time.Parse("02-Jan-2006", item.Date)
			if err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO market_price_snapshot_rows(snapshot_id, position, auction_date, auctioneer, average_rate)
				VALUES (?, ?, ?, ?, ?)`, id, i+1, auctionDate.Format("2006-01-02"), item.Auctioneer, item.AverageRate).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// RefreshAuctionPrices fetches the latest small-cardamom auction feed and saves
// its daily snapshot. It is used by both the long-running server scheduler and
// Netlify's scheduled function.
func RefreshAuctionPrices(ctx context.Context, db *gorm.DB) error {
	feed, err := fetchSmallCardamomPrices(ctx)
	if err != nil {
		return err
	}
	return saveAuctionPriceSnapshot(ctx, db, feed)
}

func latestSavedAuctionPrices(ctx context.Context, db *gorm.DB) (AuctionPriceFeed, error) {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return AuctionPriceFeed{}, err
	}
	today := time.Now().In(loc).Format("2006-01-02")
	var snapshot savedAuctionSnapshot
	err = db.WithContext(ctx).Raw("SELECT id, snapshot_date, fetched_at, source FROM market_price_snapshots WHERE snapshot_date=? LIMIT 1", today).Scan(&snapshot).Error
	if err != nil {
		return AuctionPriceFeed{}, err
	}
	feed := AuctionPriceFeed{Items: []AuctionPrice{}, Source: snapshot.Source, FetchedAt: snapshot.FetchedAt}
	if snapshot.ID == uuid.Nil {
		return feed, nil
	}
	var rows []savedAuctionPrice
	if err := db.WithContext(ctx).Raw("SELECT position, auction_date, auctioneer, average_rate::text AS average_rate FROM market_price_snapshot_rows WHERE snapshot_id=? ORDER BY position", snapshot.ID).Scan(&rows).Error; err != nil {
		return AuctionPriceFeed{}, err
	}
	for _, row := range rows {
		feed.Items = append(feed.Items, AuctionPrice{Date: row.AuctionDate.Format("02-Jan-2006"), Auctioneer: row.Auctioneer, AverageRate: row.AverageRate})
	}
	return feed, nil
}

func parseSmallCardamomPrices(doc *html.Node) []AuctionPrice {
	var tables []*html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "table" {
			tables = append(tables, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(doc)
	for _, table := range tables {
		var rows [][]string
		var collect func(*html.Node)
		collect = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "tr" {
				var cells []string
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
						cells = append(cells, strings.TrimSpace(nodeText(c)))
					}
				}
				rows = append(rows, cells)
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				collect(c)
			}
		}
		collect(table)
		hasAuctionHeaders := false
		for _, row := range rows {
			joined := strings.ToLower(strings.Join(row, " "))
			if strings.Contains(joined, "auctioneer") && strings.Contains(joined, "avg") {
				hasAuctionHeaders = true
				break
			}
		}
		if !hasAuctionHeaders {
			continue
		}
		var out []AuctionPrice
		for _, row := range rows {
			if len(row) < 9 {
				continue
			}
			if _, err := time.Parse("02-Jan-2006", row[1]); err != nil {
				continue
			}
			rate, err := strconv.ParseFloat(strings.ReplaceAll(strings.ReplaceAll(row[8], ",", ""), "₹", ""), 64)
			if err != nil {
				continue
			}
			out = append(out, AuctionPrice{Date: row[1], Auctioneer: row[2], AverageRate: fmt.Sprintf("%.2f", rate)})
			if len(out) == 3 {
				return out
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func nodeText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(nodeText(c))
	}
	return b.String()
}
