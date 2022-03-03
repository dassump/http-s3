package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	_ "github.com/joho/godotenv/autoload"
)

const (
	app_name = "http-s3"
	app_addr = "0.0.0.0"
	app_port = 3000
	app_idle = 30
	app_fork = true
)

var (
	s3_endpoint   = os.Getenv("S3_ENDPOINT")
	s3_access_key = os.Getenv("S3_ACCESS_KEY")
	s3_secret_key = os.Getenv("S3_SECRET_KEY")
	s3_secure     = os.Getenv("S3_SECURE")
	s3_bucket     = os.Getenv("S3_BUCKET")
)

func main() {
	app := fiber.New(fiber.Config{
		AppName:     app_name,
		IdleTimeout: app_idle,
		Prefork:     app_fork,
	})

	app.Use(
		recover.New(),
		logger.New(),
		cors.New(),
	)

	app.Get("*", func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), app_idle*time.Second)
		defer cancel()

		use_ssl, err := strconv.ParseBool(s3_secure)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		mc, err := minio.New(s3_endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(s3_access_key, s3_secret_key, ""),
			Secure: use_ssl,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		ok, err := mc.BucketExists(ctx, s3_bucket)
		if err != nil && !ok {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		temp, err := os.CreateTemp("", "")
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		defer os.Remove(temp.Name())

		key, err := url.PathUnescape(c.OriginalURL())
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		err = mc.FGetObject(ctx, s3_bucket, key, temp.Name(), minio.GetObjectOptions{})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		filename := strings.Split(key, "/")[len(strings.Split(key, "/"))-1]

		return c.Download(temp.Name(), filename)
	})

	go func() {
		err := app.Listen(fmt.Sprintf("%s:%d", app_addr, app_port))
		if err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	if err := app.Shutdown(); err != nil {
		log.Fatal(err)
	}
}
