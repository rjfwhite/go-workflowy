package workflowy

import (
	"net/http"
	"net/url"
	"fmt"
	"errors"
	"strings"
	"encoding/base32"
	"crypto/rand"
	"io/ioutil"
	"github.com/Jeffail/gabs"
	"log"
)

type WorkflowyClient struct {
	Session                 string
	client                  string
	most_recent_transaction string
	owner                   int
	json                    *gabs.Container
	pending_operations      [](*gabs.Container)
}

type WorkflowyItem struct {
	Id             string
	Name           string
	Completed      bool
	Priority       int
	Description    *string
	Children_names []string
}

func NewClientFromCredentials(username string, password string) (*WorkflowyClient, error) {
	session, err := login(username, password)
	if err != nil {
		return nil, err
	}
	return NewClientFromSession(session)
}

func NewClientFromSession(session string) (*WorkflowyClient, error) {
	req, err := http.NewRequest("GET", "https://workflowy-go.com/get_initialization_data?client_version=18", nil)
	req.Header.Add("Cookie", "sessionid="+session)
	resp, err := http.DefaultClient.Do(req)

	if (err != nil) {
		return nil, err
	} else {
		readall, _ := ioutil.ReadAll(resp.Body)
		json, _ := gabs.ParseJSON(readall)
		client := json.Path("projectTreeData.clientId").Data().(string)
		owner := int(json.Path("projectTreeData.mainProjectTreeInfo.ownerId").Data().(float64))
		most_recent_transaction := json.Path("projectTreeData.mainProjectTreeInfo.initialMostRecentOperationTransactionId").Data().(string)
		workflowyClient := &WorkflowyClient{Session: session, client: client, owner: owner, most_recent_transaction: most_recent_transaction, json: json, pending_operations: [](*gabs.Container){}}
		return workflowyClient, nil
	}
}

func (client *WorkflowyClient) LookupItem(path []string) (WorkflowyItem, error) {
	if client.json.ExistsP("projectTreeData.mainProjectTreeInfo.rootProjectChildren") {
		return lookupProjectNode(client.json.Path("projectTreeData.mainProjectTreeInfo.rootProjectChildren"), path)
	} else {
		return WorkflowyItem{}, errors.New("Malformed workflowy-go json")
	}
}

func lookupProjectNode(json *gabs.Container, path []string) (WorkflowyItem, error) {
	//log.Println(client.json.StringIndent("", "\t"))
	children, err := json.Children()
	if err != nil {
		return WorkflowyItem{}, errors.New("could not find children")
	}

	for priority, child := range children {
		childName := child.Path("nm").Data().(string)
		if childName == path[0] {
			log.Println("WOW FOUND " + child.Path("nm").Data().(string))

			if len(path) == 1 {
				item_id := child.Path("id").Data().(string)
				name := child.Path("nm").Data().(string)
				completed := child.Exists("cp")

				children_names := []string{}
				if child.Exists("ch") {
					metaChildren, _ := child.Path("ch").Children()
					for _, metaChild := range metaChildren {
						children_names = append(children_names, metaChild.Path("nm").Data().(string))
					}
				}

				var description *string = nil
				if child.Exists("no") {
					descriptionString := child.Path("no").Data().(string)
					description = &descriptionString
				}

				return WorkflowyItem{Id: item_id, Name: name, Priority: priority, Children_names: children_names, Completed: completed, Description: description}, nil
			} else {
				return lookupProjectNode(child.Path("ch"), path[1:])
			}
		}
	}
	return WorkflowyItem{}, errors.New("Could not find node " + path[0])
}

func (client *WorkflowyClient) AddCreate(name string, priority int, parent *string, description *string) {
	item_id := makeItemId()
	create := newCreateOperation(item_id, 10, parent)
	edit := newEditOperation(item_id, &name, description, nil, nil)
	client.pending_operations = append(client.pending_operations, create)
	client.pending_operations = append(client.pending_operations, edit)
}

func (client *WorkflowyClient) AddUpdate(item_id string, name *string, priority *int, parent *string, description *string) {
	client.pending_operations = append(client.pending_operations, newEditOperation(item_id, name, description, parent, priority))
}

func (client *WorkflowyClient) AddDelete(item_id string) {
	client.pending_operations = append(client.pending_operations, newDeleteOperation(item_id))
}

func (client *WorkflowyClient) AddComplete(item_id string) {
	client.pending_operations = append(client.pending_operations, newCompleteOperation(item_id))
}

func (client *WorkflowyClient) AddUncomplete(item_id string) {
	client.pending_operations = append(client.pending_operations, newUncompleteOperation(item_id))
}

func (client *WorkflowyClient) ApplyUpdates() error {
	form := url.Values{}

	//item_id := makeItemId()
	//create := newCreateOperation(item_id, 10, parent)
	//edit := newEditOperation(item_id, &name, nil, nil, nil)

	arry := newOperationList(client.most_recent_transaction, client.pending_operations)
	client.pending_operations = [](*gabs.Container){}

	form.Add("client_id", client.client)
	form.Add("crosscheck_user_id", string(client.owner))
	form.Add("push_poll_id", makeUpdateId())
	form.Add("client_version", "18")
	form.Add("push_poll_data", arry.String())
	log.Println(arry.StringIndent("", "\t"))

	req, _ := http.NewRequest("POST", "https://workflowy-go.com/push_and_poll", strings.NewReader(form.Encode()))
	req.Header.Add("Cookie", "sessionid="+client.Session)

	resp, err := http.DefaultClient.Do(req)

	if (err != nil) {
		return err
	}

	readall, _ := ioutil.ReadAll(resp.Body)

	log.Println(string(readall))

	log.Println(resp.StatusCode)

	return nil
}

func newOperationList(lasttxn string, operations [](*gabs.Container)) *gabs.Container {
	jsons := gabs.New()
	jsons.Set(lasttxn, "most_recent_operation_transaction_id")
	jsons.Array("operations")
	for _, operation := range operations {
		jsons.ArrayAppend(operation.Data(), "operations")
	}
	arry := gabs.New()
	arry.Array()
	arry.ArrayAppend(jsons.Data())
	return arry
}

func newOperation(item string, operation string) *gabs.Container {
	op := gabs.New()
	op.Set(operation, "type")
	//op.Set(10000, "client_timestamp")
	op.Set(item, "data", "projectid")
	return op
}

func newCreateOperation(item string, priority int, parent *string) *gabs.Container {
	parentString := "None"
	if parent != nil {
		parentString = *parent
	}
	op := newOperation(item, "create")
	op.Set(parentString, "data", "parentid")
	op.Set(priority, "data", "priority")
	return op
}

func newEditOperation(item string, name *string, description *string, parent *string, priority *int) *gabs.Container {
	op := newOperation(item, "edit")
	if name != nil {
		op.Set(*name, "data", "name")
	}
	if description != nil {
		op.Set(*description, "data", "description")
	}
	if parent != nil {
		op.Set(*parent, "data", "parentid")
	}
	if priority != nil {
		op.Set(*priority, "data", "priority")
	}
	return op
}

func newCompleteOperation(item string) *gabs.Container {
	return newOperation(item, "complete")
}

func newUncompleteOperation(item string) *gabs.Container {
	return newOperation(item, "uncomplete")
}

func newDeleteOperation(item string) *gabs.Container {
	return newOperation(item, "delete")
}

func login(username string, password string) (string, error) {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	form := url.Values{}
	form.Add("username", username)
	form.Add("password", password)

	resp, err := client.PostForm("https://workflowy-go.com/accounts/login/", form)

	if (resp.StatusCode != 302) {
		return "", err
	}

	if err != nil {
		return "", err
	} else {
		for _, cookie := range resp.Cookies() {
			if cookie.Name == "sessionid" {
				fmt.Println("EXPIRES " + cookie.Expires.String())
				return cookie.Value, nil
			}
		}
	}

	return "", errors.New("Unknown")
}

func makeItemId() (uuid string) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		fmt.Println("Error: ", err)
		return
	}
	uuid = strings.ToLower(fmt.Sprintf("%X-%X-%X-%X-%X", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]))
	return
}

func makeUpdateId() string {
	b := make([]byte, 64)
	rand.Read(b)
	return base32.StdEncoding.EncodeToString(b)[0:7]
}
