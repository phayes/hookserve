HookServe
=========

HookServe is a small golang utility for receiving github webhooks. 

```go
server := hookserve.NewServer()
server.Port = 8888
server.Secret = "supersecretcode"
server.GoListenAndServe()

for {
	select {
	case event := <-server.Events:
		fmt.Println(event.Owner + " " + event.Repo + " " + event.Branch + " " + event.Commit)
	default:
		time.Sleep(100)
	}
}
```

It also comes with a command-line utility that lets you pass webhook push events to other commands

```sh
$ hookserve logger -t PushEvent #log the push event to the system log (/var/log/message) via the logger command
```


![Configuring webhooks in github](https://i.imgur.com/u3ciUD7.png)
