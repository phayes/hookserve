package main

import (
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/phayes/hookserve/hookserve"
	"os"
	"time"
)

func main() {

	app := cli.NewApp()
	app.Name = "githubtest"
	app.Usage = "A small little application that listens for commit / push webhook events from github and runs a specified command"

	app.Flags = []cli.Flag{
		cli.IntFlag{
			Name:  "port, p",
			Value: 80,
			Usage: "port on which to listen for github webhooks",
		},
		cli.StringFlag{
			Name:  "command, c",
			Value: "",
			Usage: "command to run when a webhook is received. The command will be called like so: <command> owner repo branch commit. If no command is specified, the webhook info will be printed to stdout.",
		},
	}

	app.Action = func(c *cli.Context) {
		server := hookserve.NewServer()
		go func() {
			server.ListenAndServe()
		}()

		for {
			select {
			case commit := <-server.Events:
				fmt.Println(commit.Owner + " " + commit.Repo + " " + commit.Branch + " " + commit.Commit)
			default:
				time.Sleep(100)
			}
		}
	}

	app.Run(os.Args)
}
