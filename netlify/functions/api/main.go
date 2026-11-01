package main

import (
	"cardamom-dryer/internal/app"
	"cardamom-dryer/internal/config"
	"cardamom-dryer/internal/database"
	"encoding/base64"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/gin-gonic/gin"
)

var (
	initOnce  sync.Once
	router    *gin.Engine
	initErr   error
	initStage string
)

func initialize() {
	gin.SetMode(gin.ReleaseMode)
	initStage = "configuration"
	cfg, err := config.Load()
	if err != nil {
		initErr = err
		return
	}
	initStage = "database connection"
	db, err := database.Open(cfg.DatabaseDSN)
	if err != nil {
		initErr = err
		return
	}
	initStage = "database migrations"
	if err = database.Migrate(db); err != nil {
		initErr = err
		return
	}
	if cfg.AppEnv == "production" && cfg.InitialAdminEmail != "" && cfg.InitialAdminPassword != "" {
		initStage = "initial admin bootstrap"
		if err = app.SeedInitialAdmin(db, cfg.InitialAdminName, cfg.InitialAdminEmail, cfg.InitialAdminPassword); err != nil {
			initErr = err
			return
		}
	}
	router = app.NewRouter(db, cfg)
	initStage = ""
}

func handler(event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	initOnce.Do(func() {
		initialize()
		if initErr != nil {
			log.Printf("API initialization failed during %s: %s", initStage, redactSensitiveError(initErr.Error()))
		}
	})
	if initErr != nil {
		return jsonError(http.StatusServiceUnavailable, "backend initialization failed"), nil
	}
	path := event.Path
	if prefix := "/.netlify/functions/api"; strings.HasPrefix(path, prefix) {
		path = strings.TrimPrefix(path, prefix)
	}
	if path == "" || path == "/" {
		path = "/healthz"
	}
	if strings.HasPrefix(path, "/v1/") {
		path = "/api" + path
	}
	if !strings.HasPrefix(path, "/api/") && path != "/healthz" {
		return jsonError(http.StatusNotFound, "not found"), nil
	}
	query := url.Values{}
	for key, value := range event.QueryStringParameters {
		query.Set(key, value)
	}
	body := []byte(event.Body)
	if event.IsBase64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(event.Body)
		if err != nil {
			return jsonError(http.StatusBadRequest, "invalid request body"), nil
		}
		body = decoded
	}
	request := httptest.NewRequest(event.HTTPMethod, path+queryString(query), strings.NewReader(string(body)))
	for key, value := range event.Headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	result := events.APIGatewayProxyResponse{StatusCode: response.Code, Headers: map[string]string{}}
	for key, values := range response.Header() {
		result.Headers[key] = strings.Join(values, ", ")
	}
	result.Body, result.IsBase64Encoded = response.Body.String(), false
	return result, nil
}

func queryString(values url.Values) string {
	encoded := values.Encode()
	if encoded == "" {
		return ""
	}
	return "?" + encoded
}

func jsonError(status int, message string) events.APIGatewayProxyResponse {
	return events.APIGatewayProxyResponse{StatusCode: status, Headers: map[string]string{"Content-Type": "application/json"}, Body: `{"success":false,"error":{"message":"` + message + `"}}`}
}

func redactSensitiveError(message string) string {
	for _, key := range []string{"DATABASE_URL", "DATABASE_PASSWORD", "JWT_SECRET", "WHATSAPP_ACCESS_TOKEN", "INITIAL_ADMIN_PASSWORD"} {
		if secret := os.Getenv(key); secret != "" {
			message = strings.ReplaceAll(message, secret, "[redacted]")
		}
	}
	return message
}

func main() { lambda.Start(handler) }
