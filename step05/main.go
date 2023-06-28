// -- step05/main.go --
package main

import (
	"context"
	"errors"
	"fmt"
	jpeg "image/jpeg"
	png "image/png"
	"os"
	"path/filepath"
	"runtime"
	"runtime/trace"

	"github.com/sourcegraph/conc/panics"
	"github.com/sourcegraph/conc/pool"
)

func main() {
	ctx := context.Background()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, files []string) error {
	f, err := os.Create("trace.out")
	if err != nil {
		return err
	}
	defer f.Close()

	if err := trace.Start(f); err != nil {
		return err
	}
	defer trace.Stop()

	if err := convertAll(ctx, files); err != nil {
		return err
	}

	if err := f.Sync(); err != nil {
		return err
	}

	return nil
}

func convertAll(ctx context.Context, files []string) error {
	ctx, task := trace.NewTask(ctx, "convert all")
	defer task.End()

	pool := pool.New().WithErrors().WithMaxGoroutines(runtime.GOMAXPROCS(0)).WithContext(ctx)
	for _, file := range files {
		file := file
		pool.Go(func(ctx context.Context) (rerr error) {
			var c panics.Catcher
			defer func() {
				if r := c.Recovered(); r != nil {
					rerr = r.AsError()
				}
			}()
			c.Try(func() { rerr = convert(ctx, file) })
			return nil
		})
	}

	if err := pool.Wait(); err != nil {
		return err
	}

	return nil
}

func convert(ctx context.Context, file string) (rerr error) {
	defer trace.StartRegion(ctx, "convert "+file).End()

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	src, err := os.Open(file)
	if err != nil {
		return err
	}
	defer src.Close()
	pngimg, err := png.Decode(src)
	if err != nil {
		return err
	}

	ext := filepath.Ext(file)
	jpgfile := file[:len(file)-len(ext)] + ".jpg"

	dst, err := os.Create(jpgfile)
	if err != nil {
		return err
	}
	defer func() {
		dst.Close()
		if rerr != nil {
			rerr = errors.Join(rerr, os.Remove(jpgfile))
		}
	}()

	if err := jpeg.Encode(dst, pngimg, nil); err != nil {
		return err
	}

	if err := dst.Sync(); err != nil {
		return err
	}

	return nil
}
