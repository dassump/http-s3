package main

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"

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
		AppName: app_name,
		Prefork: app_fork,
	})

	app.Use(
		recover.New(),
		logger.New(),
		cors.New(),
	)

	app.Get("*", func(c *fiber.Ctx) error {
		mc, err := minio.New(s3_endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(s3_access_key, s3_secret_key, ""),
			Secure: strings.EqualFold(s3_secure, "true"),
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		bucket, err := mc.BucketExists(c.Context(), s3_bucket)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if !bucket {
			return fiber.ErrInternalServerError
		}

		key, err := url.PathUnescape(c.OriginalURL())
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}

		folder, file := path.Split(key)

		temp, err := os.CreateTemp("", "")
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		defer os.Remove(temp.Name())

		switch {
		case len(file) > 0:
			stat, err := mc.StatObject(c.Context(), s3_bucket, key, minio.GetObjectOptions{})
			if err != nil {
				return fiber.ErrNotFound
			}

			err = mc.FGetObject(c.Context(), s3_bucket, stat.Key, temp.Name(), minio.GetObjectOptions{})
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}

			return c.Download(temp.Name(), file)

		case len(folder) > 1:
			list := mc.ListObjects(c.Context(), s3_bucket, minio.ListObjectsOptions{Prefix: folder, Recursive: true})

			var files []string
			for v := range list {
				files = append(files, v.Key)
			}

			if len(files) < 1 {
				return fiber.ErrNotFound
			}

			zipw := zip.NewWriter(temp)

			for _, k := range files {
				obj, err := mc.GetObject(c.Context(), s3_bucket, k, minio.GetObjectOptions{})
				if err != nil {
					return fiber.NewError(fiber.StatusInternalServerError, err.Error())
				}

				zipf := strings.Replace(k, path.Dir(key)[1:], "", 1)
				if string(zipf[0]) == "/" {
					zipf = zipf[1:]
				}

				iow, err := zipw.Create(zipf)
				if err != nil {
					return fiber.NewError(fiber.StatusInternalServerError, err.Error())
				}

				if _, err := io.Copy(iow, obj); err != nil {
					return fiber.NewError(fiber.StatusInternalServerError, err.Error())
				}
				obj.Close()
			}

			_, file = path.Split(path.Clean(folder))

			if err := zipw.Close(); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}

			return c.Download(temp.Name(), file+".zip")

		default:
			return fiber.ErrNotFound
		}
	})

	go func() {
		err := app.Listen(fmt.Sprintf("%s:%d", app_addr, app_port))
		if err != nil && err != http.ErrServerClosed {
			fmt.Println(err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	if err := app.Shutdown(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
