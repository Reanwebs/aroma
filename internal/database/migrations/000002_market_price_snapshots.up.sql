CREATE TABLE market_price_snapshots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    snapshot_date date NOT NULL UNIQUE,
    fetched_at timestamptz NOT NULL,
    source text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE market_price_snapshot_rows (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    snapshot_id uuid NOT NULL REFERENCES market_price_snapshots(id) ON DELETE CASCADE,
    position integer NOT NULL,
    auction_date date NOT NULL,
    auctioneer text NOT NULL,
    average_rate numeric(12,2) NOT NULL,
    UNIQUE(snapshot_id, position)
);

CREATE INDEX market_price_snapshots_date_idx ON market_price_snapshots(snapshot_date DESC);
