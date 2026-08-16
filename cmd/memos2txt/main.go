package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"memos2txt/internal/app"
	"memos2txt/internal/cli"
	"memos2txt/internal/schema"
)

func main() {
	cfg, parseResult := cli.Parse(os.Args[1:])
	if parseResult == cli.ParseResultHelp {
		fmt.Fprint(os.Stdout, cli.HelpText())
		return
	}
	if parseResult == cli.ParseResultError {
		cli.WriteJSON(os.Stdout, schema.ErrorResponse(schema.ErrInvalidArgs, "Invalid arguments.", cfg.ParseError), cfg.JSONIndent)
		os.Exit(2)
	}

	ctx := context.Background()
	if cfg.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	resp, err := app.Run(ctx, cfg)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			resp = schema.ErrorResponse(schema.ErrTimeout, "Timed out.", err.Error())
		} else {
			resp = schema.ErrorResponse(schema.ErrProviderError, "Unexpected error.", err.Error())
		}
	}

	cli.WriteJSON(os.Stdout, resp, cfg.JSONIndent)
	if !resp.OK {
		os.Exit(1)
	}
}
