package main

import (
	"cardamom-dryer/internal/app"
	"cardamom-dryer/internal/config"
	"cardamom-dryer/internal/database"
	"context"
	"log"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

func handler(_ events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("auction refresh configuration failed: %v", err)
		return events.APIGatewayProxyResponse{StatusCode: 500}, nil
	}
	db, err := database.Open(cfg.DatabaseDSN)
	if err != nil {
		log.Printf("auction refresh database connection failed: %v", err)
		return events.APIGatewayProxyResponse{StatusCode: 500}, nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Printf("auction refresh database handle failed: %v", err)
		return events.APIGatewayProxyResponse{StatusCode: 500}, nil
	}
	defer sqlDB.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := app.RefreshAuctionPrices(ctx, db); err != nil {
		log.Printf("daily auction price refresh failed: %v", err)
		return events.APIGatewayProxyResponse{StatusCode: 500}, nil
	}
	return events.APIGatewayProxyResponse{StatusCode: 200, Body: "auction prices refreshed"}, nil
}

func main() { lambda.Start(handler) }
