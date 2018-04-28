# go-workflowy
An Unofficial Go Workflowy Client Library

Example

```go
// Login
client, _ := workflowy.NewClientFromCredentials("username", "password")

// Suggested use of sessions rather than username/password
// (can be grabbed after first username/password use)
log.Println("Logged in with sessionId %s", client.Session)

// Looking up a particular part of the workflowy item hierarchy
item, _ := client.LookupItem("My", "Workflowy", "Path")
log.Print(item.Name, item.Priority, item.Completed, item.Children_names)

// Updates are queued up, then applied
client.AddCreate("😄 My New Item", 0, nil, nil)
client.AddUpdate(item.Id, nil, nil, nil, nil)
client.AddComplete(item.Id)
client.AddUncomplete(item.Id)
client.AddDelete(item.Id)
client.ApplyAndRefresh()
```