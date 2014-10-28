HookServe
=========

HookServe is a small golang utility for receiving github webhooks. 

```go
server := hookserve.NewServer()
server.GoListenAndServe()

for {
	select {
	case commit := <-server.Events:
		fmt.Println(commit.Owner + " " + commit.Repo + " " + commit.Branch + " " + commit.Commit)
	default:
		time.Sleep(100)
	}
}
```

![Configuring webhooks in github](https://i.imgur.com/XtQSuYI.png)
