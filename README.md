# go-workflowy
An Unofficial Golang Client for [Workflowy](https://workflowy.com)

## Dependencies
https://github.com/Jeffail/gabs

## Implemented
* Login (with Session or Username / Password)
* Querying for data within the Workflowy item tree
* Updates (Create / Update / Complete / Uncomplete / Delete)

## Not Implemented
* Handling items that are shared
* Locally applying updates without re-fetching whole item tree
* Correctly honouring modification dates
* Correctly honouring undo actions
* Solid error handling

## Example
```go
// Login
client, _ := workflowy.NewClientFromCredentials("username", "password")

// Suggested use of sessions rather than username/password
// (can be grabbed after first username/password use)
log.Println("Logged in with sessionId %s", client.Session)

// Looking up a particular part of the workflowy item tree
item, _ := client.LookupItem("My", "Workflowy", "Path")
log.Print(item.Name, item.Priority, item.Completed, item.Children_names)

// Updates are queued up, then applied
client.AddCreate("😄 My New Item", 0, nil, nil)
client.AddUpdate(item.Id, nil, nil, nil, nil)
client.AddComplete(item.Id)
client.AddUncomplete(item.Id)
client.AddDelete(item.Id)

// Applies updates, and refreshes local data.
// Rather than applying the operations locally, the local data
// Is re-fetched. This will be improved in the future
client.ApplyAndRefresh()
```
