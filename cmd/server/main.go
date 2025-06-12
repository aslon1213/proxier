package main

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type ProxyJob struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	Cookies map[string]string `json:"cookies"`
	Timeout int               `json:"timeout"`
}

type ProxyResponse struct {
	StatusCode int     `json:"status_code"`
	Body       []byte  `json:"body"`
	Errs       []error `json:"errs"`
}

func PerformRequest(ctx context.Context, agent *fiber.Agent, job ProxyJob, response_chan chan ProxyResponse) {
	logger := log.With().Str("url", job.URL).Str("method", job.Method).Logger()

	for key, value := range job.Headers {
		agent.Request().Header.Set(key, value)
	}
	for key, value := range job.Cookies {
		agent.Cookie(key, value)
	}

	if job.Body != "" {
		agent.Body([]byte(job.Body))
	}

	logger.Debug().Msg("Sending request")
	status_code, body, errs := agent.Bytes()

	if len(errs) > 0 {
		logger.Error().Errs("errors", errs).Msg("Request failed")
		response_chan <- ProxyResponse{
			StatusCode: 0,
			Body:       nil,
			Errs:       errs,
		}
		return
	}

	logger.Info().Int("status_code", status_code).Int("body_size", len(body)).Msg("Request completed")
	response_chan <- ProxyResponse{
		StatusCode: status_code,
		Body:       body,
		Errs:       errs,
	}
}

func PerformProxyJob(c *fiber.Ctx) error {
	logger := log.With().Str("handler", "PerformProxyJob").Logger()

	var job ProxyJob
	if err := c.BodyParser(&job); err != nil {
		logger.Error().Err(err).Msg("Failed to parse request body")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if job.Timeout == 0 {
		job.Timeout = 30 // Default timeout of 30 seconds
	}

	logger.Info().
		Str("url", job.URL).
		Str("method", job.Method).
		Int("timeout", job.Timeout).
		Msg("Received proxy request")

	client := fiber.AcquireClient()
	defer fiber.ReleaseClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Duration(job.Timeout)*time.Second)
	defer cancel()

	var req *fiber.Agent
	switch job.Method {
	case "GET":
		req = client.Get(job.URL)
	case "POST":
		req = client.Post(job.URL)
	case "PUT":
		req = client.Put(job.URL)
	case "DELETE":
		req = client.Delete(job.URL)
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid HTTP method",
		})
	}

	response_chan := make(chan ProxyResponse, 1)
	go PerformRequest(ctx, req, job, response_chan)

	select {
	case <-ctx.Done():
		logger.Warn().Int("timeout", job.Timeout).Msg("Request timed out")
		return c.Status(fiber.StatusRequestTimeout).JSON(fiber.Map{
			"error": "Request timed out",
		})

	case response := <-response_chan:
		if len(response.Errs) > 0 {

			errors := append(response.Errs, errors.New("request timed out"))
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
				"errs": errors,
			})
		}

		logger.Info().
			Int("status_code", response.StatusCode).
			Int("body_size", len(response.Body)).
			Msg("Sending response")

		return c.Status(response.StatusCode).JSON(fiber.Map{
			"status_code": response.StatusCode,
			"body":        response.Body,
			"errs":        response.Errs,
		})
	}
}

func main() {
	// Configure zerolog
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})

	app := fiber.New()
	app.Post("/proxy", PerformProxyJob)

	log.Info().Msg("Starting server on :3010")
	log.Fatal().Err(app.Listen(":3010")).Msg("Server stopped")
}
