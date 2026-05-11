package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/app"
	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func registerImageRoutes(r *clir.Router, store *clistate.Store) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("image list", "List runtime image build targets", func(req *clir.Request) error {
			appService, _, err := openImageAppService(req, store)
			if err != nil {
				return err
			}
			targets, err := appService.RuntimeImageTargets(req.Context())
			if err != nil {
				return err
			}
			if len(targets) == 0 {
				os.Stdout.WriteString("no runtime image targets\n")
				return nil
			}
			for _, target := range targets {
				os.Stdout.WriteString(strings.Join([]string{
					target.Ref,
					"name=" + target.Name,
					"image=" + target.Image,
					"dockerfile=" + target.Dockerfile,
				}, "\t") + "\n")
			}
			return nil
		})

		b.Handle("image build all", "Build all runtime image targets", func(req *clir.Request) error {
			return buildAllRuntimeImages(req, store, req.Extra)
		})

		b.Handle("image build", "Build all runtime image targets", func(req *clir.Request) error {
			extra := append([]string(nil), req.Extra...)
			if len(extra) > 0 && strings.EqualFold(strings.TrimSpace(extra[0]), "all") {
				extra = extra[1:]
			}
			return buildAllRuntimeImages(req, store, extra)
		})
	})
}

func parseImageBuildFlags(name string, extra []string) (bool, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	noCache := fs.Bool("no-cache", false, "Build without Docker layer cache")
	if err := fs.Parse(extra); err != nil {
		return false, err
	}
	if fs.NArg() > 0 {
		return false, fmt.Errorf("unexpected %s argument: %s", name, fs.Arg(0))
	}
	return *noCache, nil
}

func buildAllRuntimeImages(req *clir.Request, store *clistate.Store, extra []string) error {
	noCache, err := parseImageBuildFlags("image build all", extra)
	if err != nil {
		return err
	}
	appService, builder, err := openImageAppService(req, store)
	if err != nil {
		return err
	}
	targets, err := appService.RuntimeImageTargets(req.Context())
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		os.Stdout.WriteString("no runtime image targets\n")
		return nil
	}
	for _, target := range targets {
		if err := builder.BuildTarget(req.Context(), target, noCache); err != nil {
			return err
		}
	}
	return nil
}

func openImageAppService(req *clir.Request, store *clistate.Store) (*app.Service, *runtimeimage.Builder, error) {
	rtSystem, err := openSystemForRoutes(req, store, nil)
	if err != nil {
		return nil, nil, err
	}
	logger := rtSystem.Logger
	if logger == nil {
		logger = log.New(os.Stdout, "", log.LstdFlags)
	}
	appService := app.NewServiceWithLogger(rtSystem.Storage, rtSystem, logger.Printf)
	builder := &runtimeimage.Builder{Logger: logger, SourceDir: rtSystem.RootDir}
	return appService, builder, nil
}
