package main

import (
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/phayes/hookserve/hookserve"
	"os"
	"os/exec"
)

func main() {

	app := cli.NewApp()
	app.Name = "hookserve"
	app.Usage = "A small little application that listens for commit / push webhook events from github and runs a specified command\n\n"
	app.Usage += "EXAMPLE:\n"
	app.Usage += "   hookserve --secret=whiskey --port=8888 echo  #Echo back the information provided\n"
	app.Usage += "   hookserve logger -t PushEvent #log the push event to the system log (/var/log/message)"
	app.Version = "1.0"
	app.Author = "Patrick Hayes"
	app.Email = "patrick.d.hayes@gmail.com"

	app.Flags = []cli.Flag{
		cli.IntFlag{
			Name:  "port, p",
			Value: 80,
			Usage: "port on which to listen for github webhooks",
		},
		cli.StringFlag{
			Name:  "secret, s",
			Value: "",
			Usage: "Secret for HMAC verification. If not provided no HMAC verification will be done and all valid requests will be processed",
		},
		cli.BoolFlag{
			Name:  "tags, t",
			Usage: "Also execute the command when a tag is pushed",
		},
	}

	app.Action = func(c *cli.Context) {
		server := hookserve.NewServer()
		server.Port = c.Int("port")
		server.Secret = c.String("secret")
		server.IgnoreTags = !c.Bool("tags")
		server.GoListenAndServe()

		for commit := range server.Events {
			if args := c.Args(); len(args) != 0 {
				root := args[0]
				rest := append(args[1:], commit.Owner, commit.Repo, commit.Branch, commit.Commit)
				cmd := exec.Command(root, rest...)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				cmd.Run()
			} else {
				fmt.Println(commit.Owner + " " + commit.Repo + " " + commit.Branch + " " + commit.Commit)
			}
		}
	}

	app.Run(os.Args)
}
