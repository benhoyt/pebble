// Copyright (c) 2021 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"

	"github.com/jessevdk/go-flags"

	"github.com/canonical/pebble/client"
)

const (
	logTimeFormat = "2006-01-02T15:04:05.000Z07:00"
)

type cmdLogs struct {
	clientMixin
	Follow     bool   `short:"f" long:"follow"`
	Format     string `long:"format"`
	N          string `short:"n"`
	Positional struct {
		Services []string `positional-arg-name:"<service>"`
	} `positional-args:"yes"`
}

var logsDescs = map[string]string{
	"follow": "Follow (tail) logs for given services until Ctrl-C pressed.",
	"format": "Output format: \"text\" (default), \"json\" (JSON lines), or \n\"raw\" (copy raw log bytes to stdout and stderr).",
	"n":      "Number of logs to show (before following); defaults to 10.\nIf 'all', show all buffered logs.",
}

var shortLogsHelp = "Fetch service logs"
var longLogsHelp = `
The logs command fetches buffered logs from the given services (or all services
if none are specified) and displays them in chronological order.
`

func (cmd *cmdLogs) Execute(args []string) error {
	var n int
	switch cmd.N {
	case "":
		n = 10
	case "all":
		n = -1
	default:
		var err error
		n, err = strconv.Atoi(cmd.N)
		if err != nil || n < 0 {
			return fmt.Errorf(`expected n to be a non-negative integer or "all", not %q`, cmd.N)
		}
	}

	var writeLog func(entry client.LogEntry) error
	switch cmd.Format {
	case "", "text":
		writeLog = func(entry client.LogEntry) error {
			suffix := ""
			if len(entry.Message) == 0 || entry.Message[len(entry.Message)-1] != '\n' {
				suffix = "\n"
			}
			_, err := fmt.Fprintf(Stdout, "%s [%s] %s%s",
				entry.Time.Format(logTimeFormat), entry.Service, entry.Message, suffix)
			return err
		}

	case "json":
		encoder := json.NewEncoder(Stdout)
		encoder.SetEscapeHTML(false)
		writeLog = func(entry client.LogEntry) error {
			return encoder.Encode(&entry)
		}

	case "raw":
		writeLog = func(entry client.LogEntry) error {
			_, err := io.WriteString(Stdout, entry.Message)
			return err
		}

	default:
		return fmt.Errorf(`invalid output format (expected "json", "text", or "raw", not %q)`, cmd.Format)
	}

	opts := client.LogsOptions{
		WriteLog: writeLog,
		Services: cmd.Positional.Services,
		N:        &n,
	}
	var err error
	if cmd.Follow {
		// Stop following when Ctrl-C pressed (SIGINT).
		ctx := notifyContext(context.Background(), os.Interrupt)
		err = cmd.client.FollowLogs(ctx, &opts)
	} else {
		err = cmd.client.Logs(&opts)
	}
	return err
}

// Needed because signal.NotifyContext is Go 1.16+
func notifyContext(parent context.Context, signals ...os.Signal) context.Context {
	ctx, cancel := context.WithCancel(parent)
	ch := make(chan os.Signal)
	signal.Notify(ch, signals...)
	go func() {
		// Wait for signal, then cancel the context.
		<-ch
		cancel()
	}()
	return ctx
}

func init() {
	addCommand("logs", shortLogsHelp, longLogsHelp, func() flags.Commander { return &cmdLogs{} }, logsDescs, nil)
}
